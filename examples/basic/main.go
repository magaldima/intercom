package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/blackrock/axis/common"
	hclog "github.com/hashicorp/go-hclog"
	"github.com/magaldima/intercom"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

var (
	kubeConfig  string // --kubeconfig
	pluginImage string // --pluginImage
	namespace   string
)

func init() {
	flag.StringVar(&kubeConfig, "kubeConfig", "", "absolute path to the kubeconfig file")
	flag.StringVar(&pluginImage, "pluginImage", "", "the image of the plugin to deploy")
	flag.StringVar(&namespace, "namespace", "default", "the namespace to deploy")
}

func main() {
	flag.Parse()

	// Create an hclog.Logger
	logger := hclog.New(&hclog.LoggerOptions{
		Name:   "plugin",
		Output: os.Stdout,
		Level:  hclog.Debug,
	})

	restConfig, err := common.GetClientConfig(kubeConfig)
	if err != nil {
		panic(err)
	}

	// create the deployment and service
	deployment := appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "greeter-plugin",
			Namespace:    namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:            "greeter",
							Image:           pluginImage,
							ImagePullPolicy: corev1.PullIfNotPresent,
						},
					},
				},
			},
		},
	}

	service := corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "greeter-plugin",
			Namespace:    namespace,
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Port:       7777,
					TargetPort: intstr.FromInt(intercom.DefaultServerPort),
				},
			},
		},
	}

	// We're a host! Start by launching the plugin process.
	client := intercom.NewClient(&intercom.ClientConfig{
		HandshakeConfig: handshakeConfig,
		Plugins:         pluginMap,
		KubeConfig:      restConfig,
		Namespace:       "default",
		Deployment:      &deployment,
		Service:         &service,
		Logger:          logger,
	})
	defer client.Kill()

	// Connect via RPC
	rpcClient, err := client.Client()
	if err != nil {
		log.Fatal(err)
	}

	// Request the plugin
	raw, err := rpcClient.Dispense("greeter")
	if err != nil {
		log.Fatal(err)
	}

	// We should have a Greeter now! This feels like a normal interface
	// implementation but is in fact over an RPC connection.
	greeter := raw.(example.Greeter)
	fmt.Println(greeter.Greet())
}

// handshakeConfigs are used to just do a basic handshake between
// a plugin and host. If the handshake fails, a user friendly error is shown.
// This prevents users from executing bad plugins or executing a plugin
// directory. It is a UX feature, not a security feature.
var handshakeConfig = intercom.HandshakeConfig{
	ProtocolVersion:  1,
	MagicCookieKey:   "BASIC_PLUGIN",
	MagicCookieValue: "hello",
}

// pluginMap is the map of plugins we can dispense.
var pluginMap = map[string]intercom.GRPCPlugin{
	"greeter": &example.GreeterPlugin{},
}

// GetClientConfig return rest config, if path not specified, assume in cluster config
func GetClientConfig(kubeconfig string) (*rest.Config, error) {
	if kubeconfig != "" {
		return clientcmd.BuildConfigFromFlags("", kubeconfig)
	}
	return rest.InClusterConfig()
}
