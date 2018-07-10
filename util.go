package intercom

import (
	"io"
	"strconv"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
)

type podLogOptions struct {
	follow    bool
	container string
	tailLines *int64
}

func newPodLogOptions(container string, follow bool, tail int) *podLogOptions {
	opts := &podLogOptions{
		container: container,
		follow:    follow,
	}

	if tail >= 0 {
		i := int64(tail)
		opts.tailLines = &i
	}
	return opts
}

// GetContainerLogs returns the remotecommand.Executor for the pod's container
func GetContainerLogs(restConfig *rest.Config, namespace string, pod string, logOptions *podLogOptions) (remotecommand.Executor, error) {
	clientset, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, err
	}

	req := clientset.CoreV1().RESTClient().Get().
		Resource("pods").
		Namespace(namespace).
		Name(pod).
		SubResource("log").
		Param("follow", strconv.FormatBool(logOptions.follow)).
		Param("container", logOptions.container)

	if logOptions.tailLines != nil {
		req.Param("tailLines", strconv.FormatInt(*logOptions.tailLines, 10))
	}

	exec, err := remotecommand.NewSPDYExecutor(restConfig, "GET", req.URL())
	if err != nil {
		return nil, err
	}
	return exec, nil
}

// ForwardContainerOutput initiates the transfer of standard shell output from the Container to the writers
func ForwardContainerOutput(exec remotecommand.Executor, stdOutWriter io.Writer, stdErrWriter io.Writer) error {
	err := exec.Stream(remotecommand.StreamOptions{
		Stdout: stdOutWriter,
		Stderr: stdErrWriter,
		Tty:    false,
	})
	if err != nil {
		return err
	}
	return nil
}
