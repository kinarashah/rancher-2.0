package statsaggregator

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/rancher/cluster-controller/controller"
	"github.com/rancher/cluster-controller/utils"
	clusterv1 "github.com/rancher/types/apis/cluster.cattle.io/v1"
	"github.com/sirupsen/logrus"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	//TODO avoid Cluster update on sync if only lastUpdateTime change, update every refreshInterval
	refreshInterval = 10 * time.Minute
	// NodeMemoryPressure means the kubelet is under pressure due to insufficient available memory.
	NodeMemoryPressure v1.NodeConditionType = "MemoryPressure"
	// NodeDiskPressure means the kubelet is under pressure due to insufficient available disk.
	NodeDiskPressure v1.NodeConditionType = "DiskPressure"
	// Pods, number
	ResourcePods v1.ResourceName = "pods"
	// CPU, in cores. (500m = .5 cores)
	ResourceCPU v1.ResourceName = "cpu"
	// Memory, in bytes. (500Gi = 500GiB = 500 * 1024 * 1024 * 1024)
	ResourceMemory v1.ResourceName = "memory"
	// ClusterConditionNoMemoryPressure true when all cluster nodes have sufficient memory
	ClusterConditionNoDiskPressure = "NoDiskPressure"
	// ClusterConditionNoMemoryPressure true when all cluster nodes have sufficient memory
	ClusterConditionNoMemoryPressure                    = "NoMemoryPressure"
	ConditionTrue                    v1.ConditionStatus = "True"
	ConditionFalse                   v1.ConditionStatus = "False"
)

type StatsAggregator struct {
	config *controller.Config
}

type ClusterNodeData struct {
	Capacity                        v1.ResourceList
	Allocatable                     v1.ResourceList
	ConditionNoDiskPressureStatus   v1.ConditionStatus
	ConditionNoMemoryPressureStatus v1.ConditionStatus
}

var stats map[string]map[string]ClusterNodeData

func init() {
	s := &StatsAggregator{}
	controller.RegisterController(s.GetName(), s)
}

func (s *StatsAggregator) GetName() string {
	return "clusterStatsAggregator"
}

func (s *StatsAggregator) Start(config *controller.Config) {
	s.config = config
	stats = make(map[string]map[string]ClusterNodeData)
	s.config.ClusterNodeController.AddHandler(s.sync)
}

func (s *StatsAggregator) sync(key string, clusterNode *clusterv1.ClusterNode) error {
	logrus.Infof("Syncing clusternode name [%s]", key)
	if clusterNode == nil {
		return s.deleteStats(key)
	} else {
		return s.addOrUpdateStats(clusterNode)
	}
}

func (s *StatsAggregator) deleteStats(key string) error {
	clusterName, clusterNodeName, err := getNames("mycluster3-minikube")
	if err != nil {
		return err
	}
	cluster, err := s.getCluster(clusterName)
	if err != nil {
		return err
	}
	if _, exists := stats[clusterName]; exists {
		delete(stats[clusterName], clusterNodeName)
	}
	logrus.Infof("ClusterNode [%s] deleted", key)
	s.aggregate(cluster, clusterName)
	err = s.update(cluster)
	if err != nil {
		return err
	}
	logrus.Infof("Successfully updated cluster [%s] stats", clusterName)
	return nil
}

func (s *StatsAggregator) addOrUpdateStats(clusterNode *clusterv1.ClusterNode) error {
	clusterName, clusterNodeName, err := getNames(clusterNode.Name)
	if err != nil {
		return err
	}
	cluster, err := s.getCluster(clusterName)
	if err != nil {
		return err
	}
	if _, exists := stats[clusterName]; !exists {
		stats[clusterName] = make(map[string]ClusterNodeData)
	}

	nodeData := ClusterNodeData{
		Capacity:                        clusterNode.Status.Capacity,
		Allocatable:                     clusterNode.Status.Allocatable,
		ConditionNoDiskPressureStatus:   getNodeConditionByType(clusterNode.Status.Conditions, NodeDiskPressure).Status,
		ConditionNoMemoryPressureStatus: getNodeConditionByType(clusterNode.Status.Conditions, NodeMemoryPressure).Status,
	}
	stats[clusterName][clusterNodeName] = nodeData
	//testing
	stats[clusterName]["kinara"] = nodeData
	s.aggregate(cluster, clusterName)
	err = s.update(cluster)
	if err != nil {
		return err
	}
	logrus.Infof("Successfully updated cluster [%s] stats", clusterName)
	return nil
}

func (s *StatsAggregator) aggregate(cluster *clusterv1.Cluster, clusterName string) {
	// capacity keys
	pods, mem, cpu := resource.Quantity{}, resource.Quantity{}, resource.Quantity{}
	// allocatable keys
	apods, amem, acpu := resource.Quantity{}, resource.Quantity{}, resource.Quantity{}

	condDisk := ConditionTrue
	condMem := ConditionTrue

	for _, v := range stats[clusterName] {
		pods.Add(*v.Capacity.Pods())
		mem.Add(*v.Capacity.Memory())
		cpu.Add(*v.Capacity.Cpu())

		apods.Add(*v.Allocatable.Pods())
		amem.Add(*v.Allocatable.Memory())
		acpu.Add(*v.Allocatable.Cpu())

		if condDisk == ConditionTrue && v.ConditionNoDiskPressureStatus == ConditionTrue {
			condDisk = ConditionFalse
		}

		if condMem == ConditionTrue && v.ConditionNoMemoryPressureStatus == ConditionTrue {
			condMem = ConditionFalse
		}
	}

	cluster.Status.Capacity = v1.ResourceList{ResourcePods: pods, ResourceMemory: mem, ResourceCPU: cpu}
	cluster.Status.Allocatable = v1.ResourceList{ResourcePods: apods, ResourceMemory: amem, ResourceCPU: acpu}

	mp(cluster.Status.Capacity, "capacity")

	utils.SetConditionStatus(cluster, ClusterConditionNoDiskPressure, condDisk)
	utils.SetConditionStatus(cluster, ClusterConditionNoMemoryPressure, condMem)
}

func (s *StatsAggregator) update(cluster *clusterv1.Cluster) error {
	_, err := s.config.ClientSet.ClusterClientV1.Clusters("").Update(cluster)
	return err
}

func (s *StatsAggregator) getCluster(clusterName string) (*clusterv1.Cluster, error) {
	return s.config.ClientSet.ClusterClientV1.Clusters("").Get(clusterName, metav1.GetOptions{})
}

func mp(i interface{}, msg string) {
	ans, _ := json.Marshal(i)
	logrus.Infof(msg+"  %s", string(ans))
}

func getNames(name string) (string, string, error) {
	splitName := strings.Split(strings.TrimSpace(name), "-")
	if len(splitName) != 2 {
		return "", "", fmt.Errorf("clusterNode name should be in the format 'node-cluster' %s", name)
	}
	clusterName := splitName[0]
	clusterNodeName := splitName[1]
	return clusterName, clusterNodeName, nil
}

func getNodeConditionByType(conditions []v1.NodeCondition, conditionType v1.NodeConditionType) *v1.NodeCondition {
	for _, condition := range conditions {
		if condition.Type == conditionType {
			return &condition
		}
	}
	return &v1.NodeCondition{}
}
