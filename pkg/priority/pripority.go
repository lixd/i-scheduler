package priority

import (
	"context"
	"fmt"
	"k8s.io/kubernetes/pkg/scheduler/framework/plugins/helper"
	"log"
	"strconv"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/kubernetes/pkg/scheduler/framework"
)

// Name is the name of the plugin used in the plugin registry and configurations.
const Name = "Priority"

const Label = "priority.lixueduan.com"

type Priority struct {
	handle framework.Handle
}

var _ framework.FilterPlugin = &Priority{}
var _ framework.ScorePlugin = &Priority{}

// New initializes a new plugin and returns it.
func New(_ context.Context, _ runtime.Object, h framework.Handle) (framework.Plugin, error) {
	return &Priority{handle: h}, nil
}

// Name returns name of the plugin.
func (pl *Priority) Name() string {
	return Name
}

func (pl *Priority) Filter(ctx context.Context, state *framework.CycleState, pod *v1.Pod, nodeInfo *framework.NodeInfo) *framework.Status {
	log.Printf("filter pod: %v, node: %v", pod.Name, nodeInfo)
	log.Println(state)

	// 只调度到携带指定 Label 的节点上
	if _, ok := nodeInfo.Node().Labels[Label]; !ok {
		return framework.NewStatus(framework.Unschedulable, fmt.Sprintf("Node:%s does not have label %s", "Node: "+nodeInfo.Node().Name, Label))
	}
	return framework.NewStatus(framework.Success, "Node: "+nodeInfo.Node().Name)
}

// Score invoked at the score extension point.
func (pl *Priority) Score(ctx context.Context, state *framework.CycleState, pod *v1.Pod, nodeName string) (int64, *framework.Status) {
	nodeInfo, err := pl.handle.SnapshotSharedLister().NodeInfos().Get(nodeName)
	if err != nil {
		return 0, framework.NewStatus(framework.Error, fmt.Sprintf("getting node %q from Snapshot: %v", nodeName, err))
	}

	// 获取 Node 上的 Label 作为分数
	priorityStr, ok := nodeInfo.Node().Labels[Label]
	if !ok {
		return 0, framework.NewStatus(framework.Error, fmt.Sprintf("node %q does not have label %s", nodeName, Label))
	}

	priority, err := strconv.Atoi(priorityStr)
	if err != nil {
		return 0, framework.NewStatus(framework.Error, fmt.Sprintf("node %q has priority %s are invalid", nodeName, priorityStr))
	}

	return int64(priority), framework.NewStatus(framework.Success, "")
}

// ScoreExtensions of the Score plugin.
func (pl *Priority) ScoreExtensions() framework.ScoreExtensions {
	return pl
}

// NormalizeScore invoked after scoring all nodes.
func (pl *Priority) NormalizeScore(ctx context.Context, state *framework.CycleState, pod *v1.Pod, scores framework.NodeScoreList) *framework.Status {
	return helper.DefaultNormalizeScore(framework.MaxNodeScore, false, scores)
}
