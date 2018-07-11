package main

import (
	"fmt"
	"os"

	hclog "github.com/hashicorp/go-hclog"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/magaldima/intercom"
	"github.com/magaldima/intercom/examples/basic/shared"
)

// various default values for the env vars
const (
	DefaultPluginImage = "mmagaldi/greeter-plugin:latest"
	DefaultNamespace   = "default"
)

func main() {
	pluginImage, ok := os.LookupEnv("pluginImage")
	if !ok {
		pluginImage = DefaultPluginImage
	}
	namespace, ok := os.LookupEnv("namespace")
	if !ok {
		namespace = DefaultNamespace
	}
	kubeConfig, _ := os.LookupEnv("kubeConfig")

	// Create an hclog.Logger
	logger := hclog.New(&hclog.LoggerOptions{
		Name:   "plugin",
		Output: os.Stdout,
		Level:  hclog.Debug,
	})

	restConfig, err := GetClientConfig(kubeConfig)
	if err != nil {
		panic(err)
	}

	labels := make(map[string]string)
	labels["plugin"] = "greeter"

	// create the deployment and service
	deployment := appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "greeter-plugin-",
			Namespace:    namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: "plugin", // TODO: source this
					Containers: []corev1.Container{
						{
							Name:            "greeter",
							Image:           pluginImage,
							ImagePullPolicy: corev1.PullIfNotPresent,
							Ports: []corev1.ContainerPort{
								{
									ContainerPort: 50077,
								},
							},
						},
					},
				},
			},
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
		},
	}

	service := corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "greeter-plugin-svc-",
			Namespace:    namespace,
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeLoadBalancer,
			Ports: []corev1.ServicePort{
				{
					Port:       7777,
					TargetPort: intstr.FromInt(intercom.DefaultServerPort),
				},
			},
			Selector: labels,
		},
	}

	// We're a host! Start by launching the plugin process.
	client := intercom.NewClient(&intercom.ClientConfig{
		HandshakeConfig: shared.Handshake,
		Plugins:         shared.PluginMap,
		KubeConfig:      restConfig,
		Namespace:       namespace,
		Deployment:      &deployment,
		Service:         &service,
		Logger:          logger,
	})
	defer client.Kill()

	// Connect via RPC
	rpcClient, err := client.Client()
	if err != nil {
		fmt.Println("Error:", err.Error())
		os.Exit(1)
	}

	// Request the plugin
	raw, err := rpcClient.Dispense("greeter")
	if err != nil {
		fmt.Println("Error:", err.Error())
		os.Exit(1)
	}

	err = rpcClient.Ping()
	if err != nil {
		fmt.Println("Error:", err.Error())
	}

	// We should have a Greeter now
	greeter := raw.(shared.Greeter)
	fmt.Println(greeter.Greet())
}

// GetClientConfig return rest config, if path not specified, assume in cluster config
func GetClientConfig(kubeconfig string) (*rest.Config, error) {
	if kubeconfig != "" {
		return clientcmd.BuildConfigFromFlags("", kubeconfig)
	}
	return rest.InClusterConfig()
}
