package statsaggregator

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/rancher/cluster-controller/controller"
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

var data map[string]map[string]*ClusterNodeData
var stats map[string]*ClusterNodeData

func init() {
	s := &StatsAggregator{}
	controller.RegisterController(s.GetName(), s)
}

func (s *StatsAggregator) GetName() string {
	return "clusterStatsAggregator"
}

func (s *StatsAggregator) Start(config *controller.Config) {
	s.config = config
	data = make(map[string]map[string]*ClusterNodeData)
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
	if _, exists := data[clusterName]; exists {
		delete(data[clusterName], clusterNodeName)
	}
	logrus.Infof("ClusterNode [%s] deleted", key)
	// s.aggregate(cluster, clusterName)
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
	if _, exists := data[clusterName]; !exists {
		data[clusterName] = make(map[string]*ClusterNodeData)
	}

	nodeData := &ClusterNodeData{
		Capacity:                        clusterNode.Status.Capacity,
		Allocatable:                     clusterNode.Status.Allocatable,
		ConditionNoDiskPressureStatus:   getNodeConditionByType(clusterNode.Status.Conditions, NodeDiskPressure).Status,
		ConditionNoMemoryPressureStatus: getNodeConditionByType(clusterNode.Status.Conditions, NodeMemoryPressure).Status,
	}
	s.aggregate(cluster, clusterName, clusterNodeName, nodeData)
	// data[clusterName][clusterNodeName] = *nodeData
	// //testing
	// data[clusterName]["kinara"] = *nodeData
	// err = s.update(cluster)
	// if err != nil {
	// 	return err
	// }
	// logrus.Infof("Successfully updated cluster [%s] stats", clusterName)
	return nil
}

func adjust(data *resource.Quantity, old *resource.Quantity, new *resource.Quantity) {
	data.Sub(*old)
	data.Add(*new)
}


func (s *StatsAggregator) aggregate(cluster *clusterv1.Cluster, clusterName string, clusterNodeName string, newData *ClusterNodeData) {
	if isResourceListChanged(newData, data[clusterName][clusterNodeName]) {
		if stat, exists := stats[clusterName]; !exists {
			stats[clusterName] = &ClusterNodeData{}
		}
		clusterStat := stats[clusterName]
		olddata := data[clusterName][clusterNodeName]

		pods, mem, cpu := clusterStat.Allocatable.Pods(), clusterStat.Allocatable.Memory(), clusterStat.Allocatable.Cpu()
		adjust(pods, olddata.Allocatable.Pods(), newData.Allocatable.Pods())
		adjust(mem, olddata.Allocatable.Memory(), newData.Allocatable.Memory())
		adjust(cpu, olddata.Allocatable.Cpu(), newData.Allocatable.Cpu())

		cpods, cmem, ccpu := clusterStat.Capacity.Pods(), clusterStat.Capacity.Memory(), clusterStat.Capacity.Cpu()
		adjust(cpods, olddata.Capacity.Pods(), newData.Capacity.Pods())
		adjust(cmem, olddata.Capacity.Memory(), newData.Capacity.Memory())
		adjust(ccpu, olddata.Capacity.Cpu(), newData.Capacity.Cpu())

		cluster.Status.Capacity = v1.ResourceList{ResourcePods: *cpods, ResourceMemory: *cmem, ResourceCPU: *ccpu}
		cluster.Status.Allocatable = v1.ResourceList{ResourcePods: *pods, ResourceMemory: *mem, ResourceCPU: *cpu}

    if isConditionChanged(new, old) {
      for _, v := range olddata[clusterName] {
        
        } 

  }
  
  
}
		condDisk := checkStatus(olddata.ConditionNoDiskPressureStatus, nodeData.ConditionNoDiskPressureStatus)
		condMem := checkStatus(ConditionNoMemoryPressureStatus, nodeData.ConditionNoMemoryPressureStatus)

		setConditionStatus(cluster, ClusterConditionNoDiskPressure, condDisk)
		setConditionStatus(cluster, ClusterConditionNoMemoryPressure, condMem)

	} else {

	}

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

func isNodeChanged(new *ClusterNodeData, old *ClusterNodeData) bool {
	return isResourceListChanged(new.Allocatable, old.Allocatable) &&
		isResourceListChanged(new.Capacity, old.Capacity) &&
		isConditionChanged(new.ConditionNoDiskPressureStatus, old.ConditionNoDiskPressureStatus) &&
		isConditionChanged(new.ConditionNoMemoryPressureStatus, old.ConditionNoMemoryPressureStatus)
}

func isResourceListChanged(m1 v1.ResourceList, m2 v1.ResourceList) bool {
	// have to assume these exist in nodes? assert in k8s
	if m1[ResourceMemory] == m2[ResourceMemory] && m1[ResourcePods] == m2[ResourcePods] && m1[ResourceCPU] == m2[ResourceCPU] {
		return false
	}
	return true
}

func isConditionChanged(new v1.ConditionStatus, old v1.ConditionStatus) bool {
	return new == old
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

func setConditionStatus(cluster *clusterv1.Cluster, conditionType clusterv1.ClusterConditionType, status v1.ConditionStatus) {
	condition := GetConditionByType(cluster, conditionType)
	now := time.Now().Format(time.RFC3339)
	if condition != nil {
		if condition.Status != status {
			condition.LastTransitionTime = now
		}
		condition.Status = status
		condition.LastHeartbeatTime = now
	}
}
