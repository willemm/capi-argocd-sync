/*

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"context"
	"strings"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	corev1 "k8s.io/api/core/v1"

	"gopkg.in/yaml.v2"
	"encoding/json"
)

var (
	ArgoCdNamespace string
)

// SecretReconciler reconciles a Secret object
type SecretReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

type KubeClusterInfo struct {
	CaData string `yaml:"certificate-authority-data"`
	Server string `yaml:"server"`
}

type KubeCluster struct {
	Name string             `yaml:"name"`
	Cluster KubeClusterInfo `yaml:"cluster"`
}

type KubeUserInfo struct {
	CertData string `yaml:"client-certificate-data"`
	KeyData  string `yaml:"client-key-data"`
}

type KubeUser struct {
	Name string       `yaml:"name"`
	User KubeUserInfo `yaml:"user"`
}

type KubeConfig struct {
	Clusters []KubeCluster `yaml:"clusters"`
	Users []KubeUser       `yaml:"users"`
}

func (kc *KubeConfig) Parse(data []byte) error {
	return yaml.Unmarshal(data, kc)
}

type ArgoTls struct {
	CaData   string `json:"caData"`
	CertData string `json:"certData"`
	KeyData  string `json:"keyData"`
}

type ArgoConfig struct {
	TlsClientConfig ArgoTls `json:"tlsClientConfig"`
}

func createArgoSecret(secretName, name, server, config string) corev1.Secret {
}

// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=secrets/status,verbs=get;update;patch

func (r *SecretReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx = context.Background()
	log = r.Log.WithValues("secret", req.NamespacedName)

	// We only care about secrets named <cluster>-kubeconfig
	if !strings.HasSuffix(req.NamespacedName.Name, "-kubeconfig") {
		return ctrl.Result{}, nil
	}

	var capiSecret corev1.Secret
	if err := r.Get(ctx, req.NamespacedName, &capiSecret); err != nil {
		log.Error("Failed to fetch secret %s", req.NamespacedName)
		return ctrl.Result{}, client.ignoreNotFound(err)
	}
	// Check if capiSecret has label cluster.x-k8s.io/cluster-name
	if !metav1.HasLabel(capiSecret.ObjectMeta, "cluster.x-k8s.io/cluster-name") {
		return ctrl.Result{}, nil
	}

	// Get kubeconfig from secret
	var kubeConfig KubeConfig
	if err := kubeConfig.Parse(capiSecret.Data['value']); err != nil {
		log.Error(err, "Failed to parse kubeconfig on")
		// If parsing failed, it's probably fatal so don't retry
		return ctrl.Result{}, nil
	}
	clusterName   := kubeConfig.Clusters[0].Name
	clusterServer := kubeConfig.Clusters[0].Cluster.Server
	caData     := kubeConfig.Clusters[0].Cluster.CaData
	certData   := kubeConfig.Users[0].User.CertData
	keyData    := kubeConfig.Users[0].User.KeyData
	if argoConfigBytes, err := json.Marshal(ArgoConfig{TlsConfig:
				ArgoTls{CaData:caData, CertData: certData, KeyData:keyData}
		}); err != nil {
		log.Error(err, "Failed to create argocd secret config")
		return ctrl.Result{}, err
	}
	clusterConfig := string(argoConfigBytes)
	argoSecretName = types.NamespacedName{Namespace=ArgoCdNamespace, Name="cluster-"+capiSecret.ObjectMeta.Name}
	var argoSecret corev1.Secret
	if err := r.Get(ctx, argoSecretName, &argoSecret); err != nil {
		if !errors.IsNotFound(err) {
			log.Error(err, "Failed to fetch secret", argoSecretName)
			return ctrl.Result{}, err
		}
		// Create new object
		argoSecret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name: argoSecretName,
				Namespace: ArgoCdNamespace
			},
			Data: map[string]string {
				"name": clusterName,
				"server": clusterServer,
				"config": clusterConfig
			}
		}
		if err := ctrl.SetControllerReference(capiSecret, argoSecret, r.Scheme); err != nil {
			log.Error(err, "Failed to set controller reference for secret", "secret", argoSecret)
			return ctrl.Result{}, err
		}
		if err := r.Create(ctx, argoSecret); err != nil {
			log.Error(err, "Failed to create argocd secret", "secret", argoSecret)
			return ctrl.Result{}, err
		}
		log.Info("Created new secret", "secret", argoSecret)
	} else {
		// Check differences and update
		changed := false
		if argoSecret.Data["name"] != clusterName {
			argoSecret.Data["name"] = clusterName
			changed = true
		}
		if argoSecret.Data["server"] != clusterServer {
			argoSecret.Data["server"] = clusterServer
			changed = true
		}
		if argoSecret.Data["config"] != clusterConfig {
			argoSecret.Data["config"] = clusterConfig
			changed = true
		}
		if changed {
			if err := r.Update(ctx, argoSecret); err != nil {
				log.Error(err, "Failed to update secret", "secret", argoSecret)
				return ctrl.Result{}, err
			}
		}
		log.Info("Updated secret", "secret", argoSecret)
	}

	return ctrl.Result{}, nil
}

func (r *SecretReconciler) SetupWithManager(mgr ctrl.Manager) error {
	ArgoCdNamespace = os.GetEnv("ARGOCD_NAMESPACE")
	if ArgoCdNamespace == "" {
		ArgoCdNamespace = "argocd"
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Secret{}).
		Complete(r)
}
