/*
Copyright 2021 Telefonaktiebolaget LM Ericsson AB

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
package generate

import (
	"application-generator/src/pkg/model"
	s "application-generator/src/pkg/service"
	"encoding/json"
	"fmt"
	"gopkg.in/yaml.v3"
	"io/ioutil"
	"os"
	"strings"
)

const (
	volumeName = "config-data-volume"
	volumePath = "/usr/src/app/config"

	imageName = "app"
	imageURL  = "app-demo:latest"

	protocol = "http"

	defaultExtPort = 80
	defaultPort    = 5000

	uri = "/"

	replicaNumber = 1

	requestsCPUDefault    = "500m"
	requestsMemoryDefault = "256M"
	limitsCPUDefault      = "1000m"
	limitsMemoryDefault   = "1024M"
)

var (
	configmap        model.ConfigMapInstance
	deployment       model.DeploymentInstance
	service          model.ServiceInstance
	serviceAccount   model.ServiceAccountInstance
	virtualService   model.VirtualServiceInstance
	workerDeployment model.DeploymentInstance
)

type CalledServices struct {
	Cluster             string  `json:"cluster"`
	Service             string  `json:"service"`
	Endpoint            string  `json:"endpoint"`
	Protocol            string  `json:"protocol"`
	TrafficForwardRatio float32 `json:"traffic_forward_ratio"`
	Requests            string  `json:"requests"`
}
type Endpoints struct {
	Name               string           `json:"name"`
	CpuConsumption     float64          `json:"cpuConsumption"`
	NetworkConsumption float64          `json:"networkConsumption"`
	MemoryConsumption  float64          `json:"memoryConsumption"`
	CalledServices     []CalledServices `json:"calledServices"`
	Requests           string           `json:"requests"`
}
type ResourceLimits struct {
	Cpu    string `json:"cpu"`
	Memory string `json:"memory"`
}
type ResourceRequests struct {
	Cpu    string `json:"cpu"`
	Memory string `json:"memory"`
}
type Resources struct {
	Limits   ResourceLimits   `json:"limits"`
	Requests ResourceRequests `json:"requests"`
}
type Services struct {
	Name      string      `json:"name"`
	Clusters  []Clusters  `json:"clusters"`
	Resources Resources   `json:"resources"`
	Endpoints []Endpoints `json:"endpoints"`
}

type Clusters struct {
	Cluster   string `json:"cluster"`
	Namespace string `json:"namespace"`
	Node      string `json:"node"`
}

type Latencies struct {
	Src     string  `json:"src"`
	Dest    string  `json:"dest"`
	Latancy float64 `json:"latency"`
}

type Config struct {
	Latencies []Latencies `json:"cluster_latencies"`
	Services  []Services  `json:"services"`
}

// NumMS the total number of the microservices in the service description file
var services, clusters, endpoints []string

// Unique return the number of unique elements in the slice of strings
func Unique(strSlice []string) []string {
	keys := make(map[string]bool)
	list := []string{}
	for _, entry := range strSlice {
		if _, value := keys[entry]; !value {
			keys[entry] = true
			list = append(list, entry)
		}
	}
	return list
}

// Parse microservice config file, and return a config struct
func Parse(configFilename string) (Config, []string) {
	configFile, err := os.Open(configFilename)
	configFileByteValue, _ := ioutil.ReadAll(configFile)

	if err != nil {
		fmt.Println(err)
	}

	var loaded_config Config
	json.Unmarshal(configFileByteValue, &loaded_config)
	for i := 0; i < len(loaded_config.Services); i++ {
		services = append(services, loaded_config.Services[i].Name)
		for j := 0; j < len(loaded_config.Services[i].Clusters); j++ {
			clusters = append(clusters, loaded_config.Services[i].Clusters[j].Cluster)
		}
		for k := 0; k < len(loaded_config.Services[i].Endpoints); k++ {
			endpoints = append(endpoints, loaded_config.Services[i].Endpoints[k].Name)
		}

	}

	fmt.Println("All clusters: ", Unique(clusters))
	fmt.Println("Number of clusters: ", len(Unique(clusters)))
	fmt.Println("---------------")
	fmt.Println("All Services: ", Unique(services))
	fmt.Println("Number of services (unique): ", len(Unique(services)))
	fmt.Println("---------------")
	fmt.Println("All endpoints: ", Unique(endpoints))
	fmt.Println("Number of endpoints: ", len(Unique(endpoints)))
	return loaded_config, clusters
}

func Create(config Config, readinessProbe int, clusters []string) {
	path, _ := os.Getwd()
	path = path + "/k8s"

	for i := 0; i < len(clusters); i++ {
		directory := fmt.Sprintf(path+"/%s", clusters[i])
		os.Mkdir(directory, 0777)
	}

	for i := 0; i < len(config.Services); i++ {
		serv := config.Services[i].Name
		resources := Resources(config.Services[i].Resources)

		if resources.Limits.Cpu == "" {
			resources.Limits.Cpu = limitsCPUDefault
		}
		if resources.Limits.Memory == "" {
			resources.Limits.Memory = limitsMemoryDefault
		}
		if resources.Requests.Cpu == "" {
			resources.Requests.Cpu = requestsCPUDefault
		}
		if resources.Requests.Memory == "" {
			resources.Requests.Memory = requestsMemoryDefault
		}

		serv_endpoints := []Endpoints(config.Services[i].Endpoints)
		serv_ep_json, err := json.Marshal(serv_endpoints)
		if err != nil {
			panic(err)
		}

		for j := 0; j < len(config.Services[i].Clusters); j++ {
			directory := config.Services[i].Clusters[j].Cluster
			directory_path := fmt.Sprintf(path+"/%s", directory)
			c_id := fmt.Sprintf("%s", config.Services[i].Clusters[j].Cluster)
			nodeAffinity := config.Services[i].Clusters[j].Node
			namespace := config.Services[i].Clusters[j].Namespace
			manifestFilePath := fmt.Sprintf(directory_path+"/%s.yaml", serv)
			manifests := make([]string, 0, 1)
			appendManifest := func(manifest interface{}) error {
				yamlDoc, err := yaml.Marshal(manifest)
				if err != nil {
					return err
				}
				manifests = append(manifests, string(yamlDoc))
				return nil
			}
			configmap = s.CreateConfig("config-"+serv, "config-"+serv, c_id, namespace, string(serv_ep_json))
			appendManifest(configmap)
			if nodeAffinity == "" {
				deployment := s.CreateDeployment(serv, serv, c_id, replicaNumber, directory, c_id, namespace,
					defaultPort, imageName, imageURL, volumePath, volumeName, "config-"+serv, readinessProbe,
					resources.Requests.Cpu, resources.Requests.Memory, resources.Limits.Cpu, resources.Limits.Memory)

				appendManifest(deployment)
			} else {
				deployment := s.CreateDeploymentWithAffinity(serv, serv, c_id, replicaNumber, directory, c_id, namespace,
					defaultPort, imageName, imageURL, volumePath, volumeName, "config-"+serv, readinessProbe,
					resources.Requests.Cpu, resources.Requests.Memory, resources.Limits.Cpu, resources.Limits.Memory, nodeAffinity)
				appendManifest(deployment)
			}

			service = s.CreateService(serv, serv, protocol, uri, c_id, namespace, defaultExtPort, defaultPort)
			appendManifest(service)

			yamlDocString := strings.Join(manifests, "---\n")
			err := ioutil.WriteFile(manifestFilePath, []byte(yamlDocString), 0644)
			if err != nil {
				fmt.Print(err)
				return
			}

		}
	}
}
