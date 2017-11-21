package healthsyncer

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	client "github.com/rancher/cluster-agent/client"
	"github.com/rancher/cluster-agent/controller"
	utils "github.com/rancher/cluster-agent/utils"
	clusterv1 "github.com/rancher/types/apis/cluster.cattle.io/v1"
	"github.com/sirupsen/logrus"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	syncInterval                    = 30 * time.Minute
	ClusterConditionProvisioned     = "Provisioned"
	ClusterConditionReady           = "Ready"
	ClusterConditionStatusHealthy   = "True"
	ClusterConditionStatusUnHealthy = "False"
)

type HealthSyncer struct {
	client      *client.ClientSet
	controller  client.ComponentStatusController
	clusterName string
}

func init() {
	h := &HealthSyncer{}
	controller.RegisterController(h.GetName(), h)
}

func (h *HealthSyncer) GetName() string {
	return "HealthSyncer"
}

func (h *HealthSyncer) Run(clusterName string, client *client.ClientSet, ctx context.Context) error {
	h.clusterName = clusterName
	h.client = client
	h.controller = client.ClusterClientV1.ComponentStatuses("").Controller()
	h.controller.AddHandler(h.sync)
	h.controller.Start(1, ctx)
	go h.syncHeartBeat(syncInterval)
	return nil
}

func (h *HealthSyncer) sync(key string, cs *v1.ComponentStatus) error {
	if cs == nil {
		//TODO : master component deleted - cluster condition Ready?
		return nil
	}
	_, err := h.client.ClusterClientV1.ComponentStatuses("").List(metav1.ListOptions{})
	if err != nil {
		logrus.Info("Got Error %v", err)
	}
	logrus.Info("Got CS")
	ans, _ := json.Marshal(cs)
	logrus.Info("%v", string(ans))
	return h.updateClusterHealth(cs)
}

func (h *HealthSyncer) updateClusterHealth(cs *v1.ComponentStatus) error {
	cluster, err := h.getCluster()
	if err != nil {
		return err
	}
	if cluster == nil {
		logrus.Info("Skip updating cluster health, cluster [%s] deleted", h.clusterName)
		return nil
	}
	// ans, _ := json.Marshal(cs)
	// logrus.Infof("Got CS %v", string(ans))
	clustercs := convertToClusterComponentStatus(cs)
	h.updateClusterStatus(cluster, clustercs)
	_, err = h.client.ClusterControllerClientV1.Clusters("").Update(cluster)
	if err != nil {
		return fmt.Errorf("Failed to update cluster [%s] %v", cluster.Name, err)
	}
	logrus.Infof("Updated cluster health successfully [%s]", cluster.Name)
	return nil
}

func (h *HealthSyncer) updateClusterStatus(cluster *clusterv1.Cluster, cs *clusterv1.ClusterComponentStatus) {
	for _, clustercs := range cluster.Status.ComponentStatuses {
		if clustercs.Name == cs.Name {
			clustercs = *cs
			return
		}
	}
	cluster.Status.ComponentStatuses = append(cluster.Status.ComponentStatuses, *cs)
}

func (h *HealthSyncer) syncHeartBeat(syncInterval time.Duration) {
	for _ = range time.Tick(syncInterval) {
		err := h.checkHeartBeat()
		if err == nil {
			logrus.Info("successfully synced hearbeat")
		} else {
			logrus.Info(err)
		}
	}
}

func (h *HealthSyncer) checkHeartBeat() error {
	cluster, err := h.getCluster()
	if err != nil {
		return err
	}
	if cluster == nil {
		return fmt.Errorf("Skip updating heartbeat, cluster [%s] deleted", h.clusterName)
	}
	// Starting heart beat only if the cluster is provisioned
	isProvisioned := utils.GetConditionByType(cluster, ClusterConditionProvisioned)
	ans, _ := json.Marshal(isProvisioned)
	logrus.Infof("Printing isProvisioned condition %v", string(ans))
	if isProvisioned == nil {
		return fmt.Errorf("Skipping heartbeat check - cluster not provisioned yet")
	}
	logrus.Info("Checking cluster API health")
	if err != nil {
		logrus.Infof("Error getting componentstatuses for server health %v", err)
		utils.UpdateConditionStatus(cluster, ClusterConditionReady, ClusterConditionStatusUnHealthy)
	} else {
		utils.UpdateConditionStatus(cluster, ClusterConditionReady, ClusterConditionStatusHealthy)
	}
	return nil
}

func (h *HealthSyncer) getCluster() (*clusterv1.Cluster, error) {
	return h.client.ClusterControllerClientV1.Clusters("").Get(h.clusterName, metav1.GetOptions{})
}

func convertToClusterComponentStatus(cs *v1.ComponentStatus) *clusterv1.ClusterComponentStatus {
	return &clusterv1.ClusterComponentStatus{
		Name:       cs.Name,
		Conditions: cs.Conditions,
	}
}
