package main

import (
	"bytes"
	"context"
	"fmt"
	"github.com/oklog/ulid/v2"
	"go.uber.org/multierr"
	"gopkg.in/yaml.v3"
	apiAppV1 "k8s.io/api/apps/v1"
	apiCoreV1 "k8s.io/api/core/v1"
	apiMetaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/intstr"
	appv1 "k8s.io/client-go/applyconfigurations/apps/v1"
	corev1 "k8s.io/client-go/applyconfigurations/core/v1"
	metav1 "k8s.io/client-go/applyconfigurations/meta/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/util/jsonpath"
	"k8s.io/kubectl/pkg/cmd/get"
	"log"
	"strconv"
	"strings"
	"text/template"
)

var configTpl = `
---
ssh:
  listen: 0.0.0.0:2222
  clientAliveInterval: 10s
  hostkeys:
  - /etc/ssh/keys/ssh_host_dsa_key
#  - /etc/ssh/keys/ssh_host_ecdsa_key
  - /etc/ssh/keys/ssh_host_ed25519_key
  - /etc/ssh/keys/ssh_host_rsa_key
security:
  forwarding:
    reverseForwardingMode: enable
    forwardingMode: enable
    socketForwardingMode: enable
    socketListenMode: enable
    x11ForwardingMode: enable
auth:
  password:
    method: passthrough
    #method: pam
  publicKey:
    method: local

backend: sshproxy

log:
  level: debug
  destination: file
  file: /var/log/containerssh.log

health:
  listen: 127.0.0.1:7002

sshproxy:
  server: {{.server_ip}}
  port: {{.server_port}}
  usernamePassThrough: true
  passwordPassThrough: true
  proxyJump:
    user: {{.jps_username}}
    password: {{.jps_password}}
    useInsecureCipher: true
    server: {{.jps_server_ip}}
    port: {{.jps_server_port}}
`

var (
	jpsUserJsonPath       = `{.spec.template.spec.containers..env[?(@.name=="USER_NAME")].value}`
	jpsPassJsonPath       = `{.spec.template.spec.containers..env[?(@.name=="USER_PASSWORD")].value}`
	jpsClusterIPJsonPath  = `{.spec.clusterIP}`
	vmiIPJsonPath         = `{.status.interfaces[0].ipAddress}`
	svcPortJsonPath       = `{.spec.ports[?(@.name=="ssh")].port}`
	svcTargetPortJsonPath = `{.spec.ports[?(@.name=="ssh")].targetPort}`
	defaultns             = "sshproxy"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	clientSet, dynamicClient, err := NewK8SClient()
	if err != nil {
		log.Fatalln(err)
	}

	log.Println("222222")
	var namespace, vmname = "wy-543162033542", "test-odv3-mp-ozhwsche"
	var name = fmt.Sprintf("sshproxy-%s", strings.ToLower(ulid.Make().String()))

	err = NewConfigMap(clientSet, dynamicClient, ctx, name, namespace, vmname)
	if err != nil {
		log.Fatalln("111111", err)
	}

	err = CreateDeploy(clientSet, ctx, name, namespace, vmname)
	if err != nil {
		log.Fatalln("222222", err)
	}
}

// CreateDeploy 生成并上传 deployment
func CreateDeploy(clientSet *kubernetes.Clientset, ctx context.Context, name, vmns, vmname string) error {
	lbs := map[string]string{
		"vm.kubevirt.io/name":       vmname,
		"vm.kubevirt.io/namespaces": vmns,
	}

	var replicas int32 = 1
	var targetPort int32 = 2222
	intString := intstr.IntOrString{
		IntVal: targetPort,
	}

	// map 转为 apiMetaV1.ListOptions 类型，即根据多个 label 查询
	labelMap, err := apiMetaV1.LabelSelectorAsMap(&apiMetaV1.LabelSelector{
		MatchLabels: lbs,
	})

	if err != nil {
		return err
	}

	options := apiMetaV1.ListOptions{
		LabelSelector: labels.SelectorFromSet(labelMap).String(),
	}

	// 如果能查到 cm 则执行 apply ，否则 create
	dps, err := clientSet.AppsV1().Deployments(defaultns).List(ctx, options)
	if err != nil {
		return err
	}
	if len(dps.Items) > 0 && err == nil {
		var merr error
		for _, item := range dps.Items {
			deployment := appv1.Deployment(item.Name, defaultns).
				WithSpec(appv1.DeploymentSpec().
					WithTemplate(
						corev1.PodTemplateSpec().
							WithSpec(
								corev1.PodSpec().
									WithContainers(
										corev1.Container().WithName("sshproxy").
											WithImage("10.185.8.245/library/nginx:1.23.2")),
							).WithLabels(lbs),
					).WithSelector(metav1.LabelSelector().WithMatchLabels(lbs)),
				)
			_, err := clientSet.AppsV1().Deployments(defaultns).Apply(ctx, deployment, apiMetaV1.ApplyOptions{FieldManager: "sshproxy-system", Force: true})
			if err != nil {
				merr = multierr.Append(nil, err)
			}
		}
		return merr
	}

	service := &apiCoreV1.Service{
		ObjectMeta: apiMetaV1.ObjectMeta{
			Name:      name,
			Labels:    lbs,
			Namespace: defaultns,
		},
		Spec: apiCoreV1.ServiceSpec{
			Type: apiCoreV1.ServiceTypeNodePort,
			Ports: []apiCoreV1.ServicePort{
				{
					Name:       "sshproxy",
					Port:       22,
					TargetPort: intString,
					//NodePort:   30088,
					Protocol: apiCoreV1.ProtocolTCP,
				},
			},
			Selector: lbs,
		},
	}

	deployment := &apiAppV1.Deployment{
		ObjectMeta: apiMetaV1.ObjectMeta{
			Name:      name,
			Labels:    lbs,
			Namespace: defaultns,
		},
		Spec: apiAppV1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &apiMetaV1.LabelSelector{
				MatchLabels: lbs,
			},
			Template: apiCoreV1.PodTemplateSpec{
				ObjectMeta: apiMetaV1.ObjectMeta{
					Name:   name,
					Labels: lbs,
				},
				Spec: apiCoreV1.PodSpec{
					Containers: []apiCoreV1.Container{
						{
							Name:  "sshproxy",
							Image: "xxxxxxxx",
							Ports: []apiCoreV1.ContainerPort{
								{
									Name:          "http",
									Protocol:      apiCoreV1.ProtocolTCP,
									ContainerPort: targetPort,
								},
							},
							VolumeMounts: []apiCoreV1.VolumeMount{
								{
									Name:      "proxy-config",
									MountPath: "/etc/containerssh",
								},
							},
						},
					},
					Volumes: []apiCoreV1.Volume{
						{
							Name: "proxy-config",
							VolumeSource: apiCoreV1.VolumeSource{
								ConfigMap: &apiCoreV1.ConfigMapVolumeSource{
									LocalObjectReference: apiCoreV1.LocalObjectReference{
										Name: name,
									},
								},
							},
						},
					},
				},
			},
		},
	}

	deployment, err = clientSet.AppsV1().Deployments(defaultns).Create(ctx, deployment, apiMetaV1.CreateOptions{})
	if err != nil {
		return err
	}

	service, err = clientSet.CoreV1().Services(defaultns).Create(ctx, service, apiMetaV1.CreateOptions{})
	if err != nil {
		return err
	}
	return nil
}

// NewConfigMap 生成并上传 configmap
func NewConfigMap(clientSet *kubernetes.Clientset, dynamicClient *dynamic.DynamicClient, ctx context.Context, name, vmns, vmname string) error {
	cfg, err := generateConfig(clientSet, dynamicClient, ctx, vmns, vmname)
	if err != nil {
		return err
	}
	//err = os.WriteFile("abc.yaml", []byte(cfg), 0666)
	//if err != nil {
	//	log.Fatalln(err)
	//}

	return configToConfigMap(clientSet, ctx, name, vmns, vmname, cfg)
}

// configToConfigMap 生成 sshproxy 的 configmap
func configToConfigMap(clientSet *kubernetes.Clientset, ctx context.Context, name, vmns, vmname, cfg string) error {
	lbs := map[string]string{
		"vm.kubevirt.io/name":       vmname,
		"vm.kubevirt.io/namespaces": vmns,
	}

	// map 转为 apiMetaV1.ListOptions 类型，即根据多个 label 查询
	labelMap, err := apiMetaV1.LabelSelectorAsMap(&apiMetaV1.LabelSelector{
		MatchLabels: lbs,
	})

	if err != nil {
		return err
	}

	options := apiMetaV1.ListOptions{
		LabelSelector: labels.SelectorFromSet(labelMap).String(),
	}

	// 如果能查到 cm 则执行 apply ，否则 create
	cms, err := clientSet.CoreV1().ConfigMaps(defaultns).List(ctx, options)
	if len(cms.Items) > 0 && err == nil {
		var merr error
		for _, item := range cms.Items {
			cm := corev1.ConfigMap(item.Name, defaultns)
			cm.Labels = lbs
			_, err := clientSet.CoreV1().ConfigMaps(defaultns).Apply(ctx, cm, apiMetaV1.ApplyOptions{FieldManager: "sshproxy-system"})
			if err != nil {
				merr = multierr.Append(merr, err)
			}
		}
		return merr
	}

	//cm := corev1.ConfigMap(fmt.Sprintf("%s.%s", vmins, vmi), defaultns).WithData(map[string]string{"sshproxy.yaml": cfg})
	cm := &apiCoreV1.ConfigMap{
		Data: map[string]string{"proxy.yaml": cfg},
		TypeMeta: apiMetaV1.TypeMeta{
			Kind:       "ConfigMap",
			APIVersion: "v1",
		},
		ObjectMeta: apiMetaV1.ObjectMeta{
			Name:      name,
			Namespace: defaultns,
			Labels:    lbs,
		},
	}
	_, err = clientSet.CoreV1().ConfigMaps(defaultns).Create(ctx, cm, apiMetaV1.CreateOptions{})
	return err
}

func generateConfig(clientSet *kubernetes.Clientset, dynamicClient *dynamic.DynamicClient, ctx context.Context, vmns, vmname string) (string, error) {
	buf := bytes.NewBuffer(nil)
	templ := template.Must(template.New("").Parse(configTpl))

	proxyDeployment, err := clientSet.AppsV1().Deployments(vmns).Get(ctx, "proxy", apiMetaV1.GetOptions{})
	if err != nil {
		return "", err
	}

	proxyService, err := clientSet.CoreV1().Services(vmns).Get(ctx, "proxy", apiMetaV1.GetOptions{})
	if err != nil {
		return "", err
	}
	users, err := jsonPath(proxyDeployment, jpsUserJsonPath)
	if err != nil {
		return "", err
	}
	pass, err := jsonPath(proxyDeployment, jpsPassJsonPath)
	if err != nil {
		return "", err
	}
	svcIP, err := jsonPath(proxyService, jpsClusterIPJsonPath)
	if err != nil {
		return "", err
	}

	// 使用 dynamic.DynamicClient{} 获取自定义 crd （这里用来获取 vm 的 CloudInit 配置）
	var gvr = schema.GroupVersionResource{
		Group:    "kubevirt.io",
		Version:  "v1",
		Resource: "virtualmachines",
	}

	vmObj, err := dynamicClient.Resource(gvr).Namespace(vmns).Get(ctx, vmname, apiMetaV1.GetOptions{})
	if err != nil {
		return "", err
	}

	userData, err := jsonPath(vmObj.Object, `{.spec.template.spec.volumes[?(@.name=="cloudinitdisk")].cloudInitNoCloud.userData}`)
	if err != nil {
		return "", err
	}

	out := struct {
		Chpasswd struct {
			Expire bool   `yaml:"expire"`
			List   string `yaml:"list"`
		} `yaml:"chpasswd"`
	}{}
	// chpasswd:
	//  list: |
	//    root:3534t(%}(+~~!+(}
	//  expire: False

	err = yaml.Unmarshal([]byte(userData[0]), &out)
	if err != nil {
		return "", err
	}

	// 获取 vmi 的 ip
	gvr = schema.GroupVersionResource{
		Group:    "kubevirt.io",
		Version:  "v1",
		Resource: "virtualmachineinstances",
	}

	vmiObj, err := dynamicClient.Resource(gvr).Namespace(vmns).Get(ctx, vmname, apiMetaV1.GetOptions{})
	if err != nil {
		return "", err
	}

	vmiIP, err := jsonPath(vmiObj.Object, vmiIPJsonPath)
	svcPort, err := jsonPath(proxyService, svcPortJsonPath)
	if err != nil {
		return "", err
	}
	svcTargetPort, err := jsonPath(proxyService, svcTargetPortJsonPath)
	if err != nil {
		return "", err
	}
	templ.Execute(buf, map[string]interface{}{
		"server_ip":       vmiIP[0],
		"server_port":     svcTargetPort[0],
		"jps_username":    users[0],
		"jps_password":    pass[0],
		"jps_server_ip":   svcIP[0],
		"jps_server_port": svcPort[0],
	})

	return strings.TrimSpace(buf.String()), nil
}

func NewK8SClient() (*kubernetes.Clientset, *dynamic.DynamicClient, error) {
	// creates the in-cluster config
	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, nil, err
	}
	config.TLSClientConfig = rest.TLSClientConfig{Insecure: true}
	// creates the clientset
	dynamicClient, err1 := dynamic.NewForConfig(config)
	err = multierr.Append(nil, err1)
	clientSet, err2 := kubernetes.NewForConfig(config)
	err = multierr.Append(nil, err2)
	return clientSet, dynamicClient, err
}

func jsonPath(obj interface{}, jpExpr string) ([]string, error) {
	// Parse and print jsonpath
	fields, err := get.RelaxedJSONPathExpression(jpExpr)
	if err != nil {
		return nil, err
	}

	j := jsonpath.New("StatusParser").AllowMissingKeys(true)
	if err := j.Parse(fields); err != nil {
		return nil, err
	}

	values, err := j.FindResults(obj)
	valueStrings := []string{}
	if len(values) == 0 || len(values[0]) == 0 {
		valueStrings = append(valueStrings, "<none>")
	}
	for arrIx := range values {
		for valIx := range values[arrIx] {
			switch val := values[arrIx][valIx].Interface().(type) {
			case int32:
				valueStrings = append(valueStrings, fmt.Sprintf("%v", strconv.FormatInt(int64(val), 10)))
			case string:
				valueStrings = append(valueStrings, fmt.Sprintf("%v", val))
			case intstr.IntOrString:
				if val.StrVal != "" {
					valueStrings = append(valueStrings, fmt.Sprintf("%v", val.StrVal))
				} else {
					valueStrings = append(valueStrings, fmt.Sprintf("%v", strconv.FormatInt(int64(val.IntVal), 10)))
				}
			}
		}
	}
	return valueStrings, nil
}
