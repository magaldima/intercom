package intercom

import (
	"io"

	apierr "k8s.io/apimachinery/pkg/api/errors"
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
func GetContainerLogs(restConfig *rest.Config, req *rest.Request) (remotecommand.Executor, error) {
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

// IsRetryableKubeAPIError returns if the error is a retryable kubernetes error
func IsRetryableKubeAPIError(err error) bool {
	if apierr.IsNotFound(err) || apierr.IsForbidden(err) || apierr.IsInvalid(err) || apierr.IsMethodNotSupported(err) {
		return false
	}
	return true
}
