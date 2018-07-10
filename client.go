package intercom

import (
	"bufio"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	hclog "github.com/hashicorp/go-hclog"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// This is a slice of the "managed" clients which are cleaned up when
// calling Cleanup
var managedClients = make([]*Client, 0, 5)
var managedClientsLock sync.Mutex

// Error types
var (
	// ErrMissingDeployment is returned when a deployment is not configured
	ErrMissingDeployment = errors.New("deployment is missing")

	// ErrMissingService is returned when a service is not configured
	// todo: have a default service?
	ErrMissingService = errors.New("service is missing")

	// ErrChecksumsDoNotMatch is returned when binary's checksum doesn't match
	// the one provided in the SecureConfig.
	ErrChecksumsDoNotMatch = errors.New("checksums did not match")

	// ErrSecureNoChecksum is returned when an empty checksum is provided to the
	// SecureConfig.
	ErrSecureConfigNoChecksum = errors.New("no checksum provided")

	// ErrSecureNoHash is returned when a nil Hash object is provided to the
	// SecureConfig.
	ErrSecureConfigNoHash = errors.New("no hash implementation provided")
)

// ClientConfig is the configuration used to initialize a new
// plugin client. After being used to initialize a plugin client,
// that configuration must not be modified again.
type ClientConfig struct {
	// HandshakeConfig is the configuration that must match servers.
	HandshakeConfig

	// Plugins are the plugins that can be consumed.
	Plugins map[string]GRPCPlugin

	// KubeConfig contains the kubernetes rest config
	KubeConfig *rest.Config

	// Namespace contains the namespace for this deployment
	Namespace string

	// Deployment contains the kubernetes deployment for this plugin
	Deployment *appsv1.Deployment

	// Service contains the kubernetes service layer for this plugin
	Service *corev1.Service

	// TLSConfig is used to enable TLS on the RPC client.
	TLSConfig *tls.Config

	// Managed represents if the client should be managed by the
	// plugin package or not. If true, then by calling CleanupClients,
	// it will automatically be cleaned up. Otherwise, the client
	// user is fully responsible for making sure to Kill all plugin
	// clients. By default the client is _not_ managed.
	Managed bool

	// StartTimeout is the timeout to wait for the plugin to say it
	// has started successfully.
	StartTimeout time.Duration

	// If non-nil, then the stderr of the client will be written to here
	// (as well as the log). This is the original os.Stderr of the subprocess.
	// This isn't the output of synced stderr.
	Stderr io.Writer

	// SyncStdout, SyncStderr can be set to override the
	// respective os.Std* values in the plugin. Care should be taken to
	// avoid races here. If these are nil, then this will automatically be
	// hooked up to os.Stdin, Stdout, and Stderr, respectively.
	//
	// If the default values (nil) are used, then this package will not
	// sync any of these streams.
	SyncStdout io.Writer
	SyncStderr io.Writer

	// Logger is the logger that the client will used. If none is provided,
	// it will default to hclog's default logger.
	Logger hclog.Logger
}

// Client handles the lifecycle of a plugin application. It deploys
// plugins, connects to them, dispenses interface implementations, and handles
// killing the containers.
//
// Plugin hosts should use one Client for each plugin executable. To
// dispense a plugin type, use the `Client.Client` function, and then
// cal `Dispense`. This awkward API is mostly historical but is used to split
// the client that deals with subprocess management and the client that
// does RPC management.
//
// See NewClient and ClientConfig for using a Client.
type Client struct {
	exited      bool
	config      *ClientConfig
	kubeClient  kubernetes.Interface
	doneLogging chan struct{}
	l           sync.Mutex
	address     net.Addr
	client      ClientProtocol
	logger      hclog.Logger
	doneCtx     context.Context
}

// NewClient creates a new plugin client which manages the lifecycle of an external
// plugin and gets the address for the RPC connection.
//
// The client must be cleaned up at some point by calling Kill(). If
// the client is a managed client (created with NewManagedClient) you
// can just call CleanupClients at the end of your program and they will
// be properly cleaned.
func NewClient(config *ClientConfig) (c *Client) {
	if config.StartTimeout == 0 {
		config.StartTimeout = 1 * time.Minute
	}

	if config.Stderr == nil {
		config.Stderr = ioutil.Discard
	}

	if config.SyncStdout == nil {
		config.SyncStdout = ioutil.Discard
	}
	if config.SyncStderr == nil {
		config.SyncStderr = ioutil.Discard
	}

	if config.Logger == nil {
		config.Logger = hclog.New(&hclog.LoggerOptions{
			Output: hclog.DefaultOutput,
			Level:  hclog.Trace,
			Name:   "plugin",
		})
	}

	k8Interface := kubernetes.NewForConfigOrDie(config.KubeConfig)

	c = &Client{
		config:     config,
		kubeClient: k8Interface,
		logger:     config.Logger,
	}
	if config.Managed {
		managedClientsLock.Lock()
		managedClients = append(managedClients, c)
		managedClientsLock.Unlock()
	}
	return
}

// Client returns the protocol client for this connection.
//
// Subsequent calls to this will return the same client.
func (c *Client) Client() (ClientProtocol, error) {
	_, err := c.Start()
	if err != nil {
		return nil, err
	}

	c.l.Lock()
	defer c.l.Unlock()

	if c.client != nil {
		return c.client, nil
	}

	c.client, err = newGRPCClient(c.doneCtx, c)

	if err != nil {
		c.client = nil
		return nil, err
	}

	return c.client, nil
}

// Start the deployment and the service, communicating with it to negotiate
// a port for RPC connections, and returning the address to connect via RPC.
//
// This method is safe to call multiple times. Subsequent calls have no effect.
// Once a client has been started once, it cannot be started again, even if
// it was killed.
func (c *Client) Start() (addr net.Addr, err error) {
	c.l.Lock()
	defer c.l.Unlock()

	if c.address != nil {
		return c.address, nil
	}

	// If deployment or service isn't set, then it is an error. We wrap
	// this in a {} for scoping reasons, and hopeful that the escape
	// analysis will pop the stock here.
	{
		if c.config.Deployment == nil {
			return nil, ErrMissingDeployment
		}
		if c.config.Service == nil {
			return nil, ErrMissingService
		}
	}

	err = ValidateDeployment(c.config.Deployment)
	if err != nil {
		return nil, err
	}
	err = ValidateService(c.config.Service)
	if err != nil {
		return nil, err
	}

	// Create the logging channel for when we kill
	c.doneLogging = make(chan struct{})
	// Create a context for when we kill
	var ctxCancel context.CancelFunc
	c.doneCtx, ctxCancel = context.WithCancel(context.Background())

	cookieEnv := corev1.EnvVar{
		Name:  c.config.MagicCookieKey,
		Value: c.config.MagicCookieValue,
	}

	deployments := c.kubeClient.AppsV1().Deployments(c.config.Namespace)
	services := c.kubeClient.CoreV1().Services(c.config.Namespace)

	deployment := *c.config.Deployment
	service := *c.config.Service

	// todo: apply label overrides to deployment and service

	// apply overrides to the deployment and service
	service.ObjectMeta.OwnerReferences = []metav1.OwnerReference{
		*metav1.NewControllerRef(&deployment, appsv1.SchemeGroupVersion.WithKind("Deployment")),
	}

	// apply an environment variable to all deployment's containers
	for _, container := range deployment.Spec.Template.Spec.Containers {
		if container.Env == nil {
			container.Env = make([]corev1.EnvVar, 0)
		}
		container.Env = append(container.Env, cookieEnv)
		container.Stdin = true
		//container.StdinOnce = true
	}

	// create the deployment and service
	createdDeployment, err := deployments.Create(&deployment)
	if err != nil {
		return nil, fmt.Errorf("failed to create deployment '%s': %s", deployment.Name, err)
	}
	createdService, err := services.Create(&service)
	if err != nil {
		return nil, fmt.Errorf("failed to create service '%s': %s", service.Name, err)
	}

	c.logger.Debug("starting plugin")
	addr, err = net.ResolveTCPAddr("tcp", createdService.Name)
	if err != nil {
		c.logger.Warn("failed to resolve tcp address from service name '%s': %s", createdService.Name, err)
	}

	stdoutR, stdoutW := io.Pipe()
	stderrR, stderrW := io.Pipe()

	podName := createdDeployment.Spec.Template.Name
	containerName := createdDeployment.Spec.Template.Spec.Containers[0].Name

	// TODO: ideally we should iterate over the retrieved pods after we list according to some listOpts using labels
	logs, err := GetContainerLogs(c.config.KubeConfig, c.config.Namespace, podName, newPodLogOptions(containerName, true, 0))
	if err != nil {
		return nil, fmt.Errorf("failed to get container '%s' logs: %s", containerName, err)
	}

	err = ForwardContainerOutput(logs, stdoutW, stderrW)
	if err != nil {
		return nil, fmt.Errorf("failed to stream container '%s' logs: %s", containerName, err)
	}

	// Make sure the command is properly cleaned up if there is an error
	defer func() {
		r := recover()

		if err != nil || r != nil {
			deployments.Delete(deployment.Name, &metav1.DeleteOptions{})
		}

		if r != nil {
			panic(r)
		}
	}()

	// Start goroutine to wait for deployment to exit
	exitCh := make(chan struct{})
	go func() {
		// Make sure we close the write end of our stderr/stdout so
		// that the readers send EOF properly.
		defer stderrW.Close()
		defer stdoutW.Close()

		// Wait for the deployment to be destroyed
		waitCondition := func() (done bool, err error) {
			obj, err := deployments.Get(deployment.Name, metav1.GetOptions{})
			if err != nil {
				return false, err
			}
			if obj == nil {
				return true, nil
			}
			return false, nil
		}
		wait.PollUntil(time.Second*10, waitCondition, c.doneCtx.Done())

		// Log and make sure to flush the logs write away
		c.logger.Debug("plugin deployment exited")
		os.Stderr.Sync()

		// Mark that we exited
		close(exitCh)

		// Cancel the context, marking that we exited
		ctxCancel()

		// Set that we exited, which takes a lock
		c.l.Lock()
		defer c.l.Unlock()
		c.exited = true
	}()

	// Start goroutine that logs the stderr
	go c.logStderr(stderrR)

	// Start a goroutine that is going to be reading the lines
	// out of stdout
	linesCh := make(chan []byte)
	go func() {
		defer close(linesCh)

		buf := bufio.NewReader(stdoutR)
		for {
			line, err := buf.ReadBytes('\n')
			if line != nil {
				linesCh <- line
			}

			if err == io.EOF {
				return
			}
		}
	}()

	// Make sure after we exit we read the lines from stdout forever
	// so they don't block since it is an io.Pipe
	defer func() {
		go func() {
			for _ = range linesCh {
			}
		}()
	}()

	// Some channels for the next step
	timeout := time.After(c.config.StartTimeout)

	// Start looking for the address
	c.logger.Debug("waiting for RPC address")
	select {
	case <-timeout:
		err = errors.New("timeout while waiting for plugin to start")
	case <-exitCh:
		err = errors.New("plugin exited before we could connect")
	case lineBytes := <-linesCh:
		// Trim the line and split by "|" in order to get the parts of
		// the output.
		line := strings.TrimSpace(string(lineBytes))
		parts := strings.SplitN(line, "|", 5)
		if len(parts) < 4 {
			err = fmt.Errorf(
				"Unrecognized remote plugin message: %s\n\n"+
					"This usually means that the plugin is either invalid or simply\n"+
					"needs to be recompiled to support the latest protocol.", line)
			return
		}

		// Check the core protocol. Wrapped in a {} for scoping.
		{
			var coreProtocol int64
			coreProtocol, err = strconv.ParseInt(parts[0], 10, 0)
			if err != nil {
				err = fmt.Errorf("Error parsing core protocol version: %s", err)
				return
			}

			if int(coreProtocol) != CoreProtocolVersion {
				err = fmt.Errorf("Incompatible core API version with plugin. "+
					"Plugin version: %s, Core version: %d\n\n"+
					"To fix this, the plugin usually only needs to be recompiled.\n"+
					"Please report this to the plugin author.", parts[0], CoreProtocolVersion)
				return
			}
		}

		// Parse the protocol version
		var protocol int64
		protocol, err = strconv.ParseInt(parts[1], 10, 0)
		if err != nil {
			err = fmt.Errorf("Error parsing protocol version: %s", err)
			return
		}

		// Test the API version
		if uint(protocol) != c.config.ProtocolVersion {
			err = fmt.Errorf("Incompatible API version with plugin. "+
				"Plugin version: %s, Core version: %d", parts[1], c.config.ProtocolVersion)
			return
		}

		switch parts[2] {
		case "tcp":
			addr, err = net.ResolveTCPAddr("tcp", parts[3])
			break
		default:
			err = fmt.Errorf("Unknown address type: %s", parts[3])
			return
		}
	}

	c.address = addr
	return
}

// Exited tells whether or not the deployment has been destroyed.
func (c *Client) Exited() bool {
	c.l.Lock()
	defer c.l.Unlock()
	return c.exited
}

// Kill the executing deployment and associated pods and perform any cleanup
// tasks necessary such as capturing any remaining logs and so on.
//
// This method blocks until the deployment+service is successfully destroyed.
//
// This method can safely be called multiple times.
func (c *Client) Kill() {
	deployments := c.kubeClient.AppsV1().Deployments(c.config.Namespace)
	deployment, err := deployments.Get(c.config.Deployment.Name, metav1.GetOptions{})
	if err != nil {
		c.logger.Warn("failed getting deployment '%s'. nothing to kill.", deployment.Name)
		return
	}

	// Grab a lock to read some private fields.
	c.l.Lock()
	addr := c.address
	doneCh := c.doneLogging
	c.l.Unlock()

	// If there is no process, we never started anything. Nothing to kill.
	if deployment == nil {
		return
	}

	// We need to check for address here. It is possible that the plugin
	// started (process != nil) but has no address (addr == nil) if the
	// plugin failed at startup. If we do have an address, we need to close
	// the plugin net connections.
	graceful := false
	if addr != nil {
		// Close the client to cleanly exit the process.
		client, err := c.Client()
		if err == nil {
			err = client.Close()

			// If there is no error, then we attempt to wait for a graceful
			// exit. If there was an error, we assume that graceful cleanup
			// won't happen and just force kill.
			graceful = err == nil
			if err != nil {
				// If there was an error just log it. We're going to force
				// kill in a moment anyways.
				c.logger.Warn("error closing client during Kill", "err", err)
			}
		}
	}

	// If we're attempting a graceful exit, then we wait for a short period
	// of time to allow that to happen. To wait for this we just wait on the
	// doneCh which would be closed if the process exits.
	if graceful {
		select {
		case <-doneCh:
			return
		case <-time.After(250 * time.Millisecond):
		}
	}

	// If graceful exiting failed, just kill it

	deployments.Delete(c.config.Deployment.Name, &metav1.DeleteOptions{})

	// Wait for the client to finish logging so we have a complete log
	<-doneCh
}

func netAddrDialer(addr net.Addr) func(string, time.Duration) (net.Conn, error) {
	return func(_ string, _ time.Duration) (net.Conn, error) {
		// Connect to the client
		conn, err := net.Dial(addr.Network(), addr.String())
		if err != nil {
			return nil, err
		}
		if tcpConn, ok := conn.(*net.TCPConn); ok {
			// Make sure to set keep alive so that the connection doesn't die
			tcpConn.SetKeepAlive(true)
		}

		return conn, nil
	}
}

// dialer is compatible with grpc.WithDialer and creates the connection
// to the plugin.
func (c *Client) dialer(_ string, timeout time.Duration) (net.Conn, error) {
	conn, err := netAddrDialer(c.address)("", timeout)
	if err != nil {
		return nil, err
	}

	return conn, nil
}

func (c *Client) logStderr(r io.Reader) {
	bufR := bufio.NewReader(r)
	l := c.logger.Named(c.config.Deployment.Name)

	for {
		line, err := bufR.ReadString('\n')
		if line != "" {
			c.config.Stderr.Write([]byte(line))
			line = strings.TrimRightFunc(line, unicode.IsSpace)

			entry, err := parseJSON(line)
			// If output is not JSON format, print directly to Debug
			if err != nil {
				l.Debug(line)
			} else {
				out := flattenKVPairs(entry.KVPairs)

				out = append(out, "timestamp", entry.Timestamp.Format(hclog.TimeFormat))
				switch hclog.LevelFromString(entry.Level) {
				case hclog.Trace:
					l.Trace(entry.Message, out...)
				case hclog.Debug:
					l.Debug(entry.Message, out...)
				case hclog.Info:
					l.Info(entry.Message, out...)
				case hclog.Warn:
					l.Warn(entry.Message, out...)
				case hclog.Error:
					l.Error(entry.Message, out...)
				}
			}
		}

		if err == io.EOF {
			break
		}
	}

	// Flag that we've completed logging for others
	close(c.doneLogging)
}
