package main

import (
	"flag"
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

var (
	kubeConfig  string // --kubeconfig
	pluginImage string // --pluginImage
	namespace   string // --namespace
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

	restConfig, err := GetClientConfig(kubeConfig)
	if err != nil {
		panic(err)
	}

	labels := make(map[string]string)
	labels["plugin"] = "kv"

	// create the deployment and service
	deployment := appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "kv-plugin-",
			Namespace:    namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:            "kv",
							Image:           pluginImage,
							ImagePullPolicy: corev1.PullIfNotPresent,
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
			GenerateName: "kv-plugin-",
			Namespace:    namespace,
		},
		Spec: corev1.ServiceSpec{
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
		Namespace:       "default",
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
	raw, err := rpcClient.Dispense("kv")
	if err != nil {
		fmt.Println("Error:", err.Error())
		os.Exit(1)
	}

	// We should have a KV store now! This feels like a normal interface
	// implementation but is in fact over an RPC connection.
	kv := raw.(shared.KV)
	os.Args = os.Args[1:]
	switch os.Args[0] {
	case "get":
		result, err := kv.Get(os.Args[1])
		if err != nil {
			fmt.Println("Error:", err.Error())
			os.Exit(1)
		}

		fmt.Println(string(result))

	case "put":
		err := kv.Put(os.Args[1], []byte(os.Args[2]))
		if err != nil {
			fmt.Println("Error:", err.Error())
			os.Exit(1)
		}

	default:
		fmt.Println("Please only use 'get' or 'put'")
		os.Exit(1)
	}
}

// GetClientConfig return rest config, if path not specified, assume in cluster config
func GetClientConfig(kubeconfig string) (*rest.Config, error) {
	if kubeconfig != "" {
		return clientcmd.BuildConfigFromFlags("", kubeconfig)
	}
	return rest.InClusterConfig()
}
