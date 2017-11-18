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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	//TODO avoid Cluster update on sync if only lastUpdateTime change, update every refreshInterval
	refreshInterval = 10 * time.Minute
	// Pods, number
	ResourcePods v1.ResourceName = "pods"
	// CPU, in cores. (500m = .5 cores)
	ResourceCPU v1.ResourceName = "cpu"
	// Memory, in bytes. (500Gi = 500GiB = 500 * 1024 * 1024 * 1024)
	ResourceMemory v1.ResourceName = "memory"
	// NodeMemoryPressure means the kubelet is under pressure due to insufficient available memory.
	NodeMemoryPressure v1.NodeConditionType = "MemoryPressure"
	// NodeDiskPressure means the kubelet is under pressure due to insufficient available disk.
	NodeDiskPressure v1.NodeConditionType = "DiskPressure"
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
		//TODO: deleteClusterNode
	} else {
		return s.createOrUpdateClusterStats(clusterNode)
	}
	return nil
}

func (s *StatsAggregator) createOrUpdateClusterStats(clusterNode *clusterv1.ClusterNode) error {
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
	storedNodeStats := stats[clusterName][clusterNodeName]
	mp(storedNodeStats, "storedNodeStats")

	if isNodeChanged(&storedNodeStats, clusterNode) {
		logrus.Infof("node changed!")
		nodeData := ClusterNodeData{
			Capacity:                        clusterNode.Status.Capacity,
			Allocatable:                     clusterNode.Status.Allocatable,
			ConditionNoDiskPressureStatus:   utils.GetNodeConditionByType(clusterNode.Status.Conditions, NodeDiskPressure).Status,
			ConditionNoMemoryPressureStatus: utils.GetNodeConditionByType(clusterNode.Status.Conditions, NodeMemoryPressure).Status,
		}
    
		stats[clusterName][clusterNodeName] = nodeData
	} else {
    
  }
	return nil
}

func (s *StatsAggregator) aggregate(cluster *clusterv1.Cluster) {

}

func (s *StatsAggregator) getCluster(clusterName string) (*clusterv1.Cluster, error) {
	return s.config.ClientSet.ClusterClientV1.Clusters("").Get(clusterName, metav1.GetOptions{})
}

func mp(i interface{}, msg string) {
	ans, _ := json.Marshal(i)
	logrus.Infof(msg+"  %s", string(ans))
}

func isNodeChanged(stored *ClusterNodeData, new *clusterv1.ClusterNode) bool {
	return isResourceListChanged(stored.Allocatable, new.Status.Allocatable) &&
		isResourceListChanged(stored.Capacity, new.Status.Capacity) &&
		// only required types status
		isConditionListChanged(stored.ConditionNoDiskPressureStatus, new.Status.Conditions, NodeDiskPressure) &&
		isConditionListChanged(stored.ConditionNoMemoryPressureStatus, new.Status.Conditions, NodeMemoryPressure)
}

func isResourceListChanged(m1 v1.ResourceList, m2 v1.ResourceList) bool {
	// have to assume these exist in nodes? assert in k8s
	if m1[ResourceMemory] == m2[ResourceMemory] && m1[ResourcePods] == m2[ResourcePods] && m1[ResourceCPU] == m2[ResourceCPU] {
		return false
	}
	return true
}

func isConditionListChanged(oldstatus v1.ConditionStatus, c2 []v1.NodeCondition, nodeType v1.NodeConditionType) bool {
	c := utils.GetNodeConditionByType(c2, nodeType)
	if oldstatus == c.Status {
		return false
	}
	return true
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
