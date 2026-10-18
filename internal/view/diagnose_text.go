// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of helmsman

package view

import (
	"fmt"
	"sort"
	"strings"

	"github.com/derailed/tview"
	"github.com/rsmrtk/helmsman/internal/model1"
	v1 "k8s.io/api/core/v1"
)

const (
	diagTail     = 30 // lines of previous logs per container
	diagEvents   = 8
	findingWidth = 150
	tagHeading   = "[::b]"
	tagReset     = "[-::-]"
	tagDim       = "[::d]"
	diagUnknown  = "-"
)

func statusTag(status string) string {
	if c, ok := model1.StatusColor(status); ok {
		return fmt.Sprintf("[#%06x::b]", c.TrueColor().Hex())
	}

	return tagHeading
}

// podPhase is the status word kubectl would show for the pod, simplified to
// what the palette knows about.
func podPhase(pod *v1.Pod) string {
	if pod.DeletionTimestamp != nil {
		return "Terminating"
	}
	for _, cs := range pod.Status.ContainerStatuses {
		if w := cs.State.Waiting; w != nil && w.Reason != "" {
			return w.Reason
		}
		if t := cs.State.Terminated; t != nil && t.Reason != "" && t.Reason != "Completed" {
			return t.Reason
		}
	}
	for _, cs := range pod.Status.InitContainerStatuses {
		if w := cs.State.Waiting; w != nil && w.Reason != "" && w.Reason != "PodInitializing" {
			return "Init:" + w.Reason
		}
		if t := cs.State.Terminated; t != nil && t.ExitCode != 0 {
			return "Init:Error"
		}
	}
	if pod.Status.Reason != "" {
		return pod.Status.Reason
	}

	return string(pod.Status.Phase)
}

// advice maps a failure reason to what to look at first.
func advice(reason string) string {
	switch reason {
	case "ImagePullBackOff", "ErrImagePull":
		return "the image cannot be pulled: check the name and tag, that the registry is reachable, and imagePullSecrets for private registries"
	case "InvalidImageName":
		return "the image reference is malformed: check the image field for spaces, a bad tag or a missing registry"
	case "CrashLoopBackOff":
		return "the container keeps exiting: read the previous logs below and the exit code under Containers"
	case "OOMKilled":
		return "the container hit its memory limit: raise resources.limits.memory or fix a leak"
	case "CreateContainerConfigError", "CreateContainerError":
		return "the container could not be configured: a referenced ConfigMap, Secret or key is probably missing"
	case "Error":
		return "the container exited with an error: see the exit code and the logs below"
	case "Evicted":
		return "the node evicted the pod, usually for memory or disk pressure: check the node conditions"
	case "DeadlineExceeded":
		return "the pod ran past activeDeadlineSeconds"
	}

	return ""
}

type finding struct{ severity, text, advice string }

func findings(pod *v1.Pod, events []v1.Event) []finding {
	var ff []finding
	add := func(sev, format string, a ...any) {
		ff = append(ff, finding{severity: sev, text: fmt.Sprintf(format, a...)})
	}
	addAdvice := func(sev, advice, format string, a ...any) {
		ff = append(ff, finding{severity: sev, text: fmt.Sprintf(format, a...), advice: advice})
	}

	for _, c := range pod.Status.Conditions {
		if c.Type == v1.PodScheduled && c.Status == v1.ConditionFalse {
			add("CrashLoopBackOff", "not scheduled: %s", firstNonEmpty(c.Message, c.Reason))
		}
	}
	for _, group := range [][]v1.ContainerStatus{pod.Status.InitContainerStatuses, pod.Status.ContainerStatuses} {
		for _, cs := range group {
			switch {
			case cs.State.Waiting != nil && cs.State.Waiting.Reason != "" && cs.State.Waiting.Reason != "PodInitializing":
				w := cs.State.Waiting
				addAdvice(w.Reason, advice(w.Reason), "container %q is waiting: %s", cs.Name, firstNonEmpty(w.Reason))
			case cs.State.Terminated != nil && cs.State.Terminated.ExitCode != 0:
				t := cs.State.Terminated
				addAdvice(firstNonEmpty(t.Reason, "Error"), advice(t.Reason), "container %q exited with code %d (%s)", cs.Name, t.ExitCode, firstNonEmpty(t.Reason, "no reason"))
			}
			if t := cs.LastTerminationState.Terminated; t != nil && t.ExitCode != 0 && cs.RestartCount > 0 {
				add(firstNonEmpty(t.Reason, "Error"), "container %q was last terminated: %s, exit code %d, %d restart(s)", cs.Name, firstNonEmpty(t.Reason, "Error"), t.ExitCode, cs.RestartCount)
			}
		}
	}
	if pod.Status.Phase == v1.PodRunning {
		for _, cs := range pod.Status.ContainerStatuses {
			if !cs.Ready && cs.State.Running != nil {
				add("NoReady", "container %q is running but not ready: its readiness probe is failing or has not passed yet", cs.Name)
			}
		}
	}
	for _, e := range events {
		if e.Reason == "FailedMount" || e.Reason == "FailedAttachVolume" {
			add("CreateContainerConfigError", "volume problem: %s", strings.TrimSpace(e.Message))
		}
	}
	for _, c := range pod.Spec.Containers {
		if len(c.Resources.Requests) == 0 && len(c.Resources.Limits) == 0 {
			add("Pending", "container %q sets no requests or limits, so it is the first to be evicted and cannot be scheduled by size", c.Name)
		}
	}

	return ff
}

func firstNonEmpty(ss ...string) string {
	for _, s := range ss {
		if strings.TrimSpace(s) != "" {
			return strings.TrimSpace(s)
		}
	}

	return diagUnknown
}

// diagnoseText builds the diagnosis of one pod: findings with advice, every
// container's state, the latest events and the tail of the previous logs.
func diagnoseText(pod *v1.Pod, events []v1.Event, prevLogs map[string]string) string {
	var sb strings.Builder
	phase := podPhase(pod)

	fmt.Fprintf(&sb, "%s%s/%s%s  %s%s%s\n", tagHeading, pod.Namespace, pod.Name, tagReset, statusTag(phase), phase, tagReset)
	fmt.Fprintf(&sb, "%snode %s  ·  ip %s  ·  qos %s%s\n\n", tagDim, firstNonEmpty(pod.Spec.NodeName), firstNonEmpty(pod.Status.PodIP), pod.Status.QOSClass, tagReset)

	ff := findings(pod, events)
	fmt.Fprintf(&sb, "%sFindings%s\n", tagHeading, tagReset)
	if len(ff) == 0 {
		fmt.Fprintf(&sb, "%s●%s nothing wrong found\n", statusTag("Running"), tagReset)
	}
	for _, f := range ff {
		fmt.Fprintf(&sb, "%s●%s %s\n", statusTag(f.severity), tagReset, tview.Escape(truncate(oneLine(f.text), findingWidth)))
		if f.advice != "" {
			fmt.Fprintf(&sb, "  %s→ %s%s\n", tagDim, tview.Escape(f.advice), tagReset)
		}
	}

	fmt.Fprintf(&sb, "\n%sContainers%s\n", tagHeading, tagReset)
	all := append(append([]v1.ContainerStatus{}, pod.Status.InitContainerStatuses...), pod.Status.ContainerStatuses...)
	for _, cs := range all {
		state, color := "running", "Running"
		switch {
		case cs.State.Waiting != nil:
			state, color = "waiting: "+firstNonEmpty(cs.State.Waiting.Reason), cs.State.Waiting.Reason
		case cs.State.Terminated != nil:
			state, color = fmt.Sprintf("terminated: %s (exit %d)", firstNonEmpty(cs.State.Terminated.Reason), cs.State.Terminated.ExitCode), firstNonEmpty(cs.State.Terminated.Reason)
		}
		fmt.Fprintf(&sb, "%s●%s %-18s %s  ready=%v restarts=%d  image=%s\n", statusTag(color), tagReset, cs.Name, state, cs.Ready, cs.RestartCount, cs.Image)
	}

	fmt.Fprintf(&sb, "\n%sEvents%s\n", tagHeading, tagReset)
	evs := append([]v1.Event{}, events...)
	sort.SliceStable(evs, func(i, j int) bool {
		// warnings first, then the most recent
		wi, wj := evs[i].Type == v1.EventTypeWarning, evs[j].Type == v1.EventTypeWarning
		if wi != wj {
			return wi
		}
		return evs[j].LastTimestamp.Before(&evs[i].LastTimestamp)
	})
	if len(evs) > diagEvents {
		evs = evs[:diagEvents]
	}
	if len(evs) == 0 {
		fmt.Fprintf(&sb, "%sno events%s\n", tagDim, tagReset)
	}
	for _, e := range evs {
		tag, mark := tagDim, "○"
		if e.Type == v1.EventTypeWarning {
			tag, mark = statusTag("CrashLoopBackOff"), "●"
		}
		fmt.Fprintf(&sb, "%s%s %s%s\n", tag, mark, e.Reason, tagReset)
		fmt.Fprintf(&sb, "    %s%s%s\n", tagDim, tview.Escape(truncate(oneLine(e.Message), findingWidth)), tagReset)
	}

	names := make([]string, 0, len(prevLogs))
	for n := range prevLogs {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		fmt.Fprintf(&sb, "\n%sPrevious logs: %s%s (last %d lines)\n", tagHeading, n, tagReset, diagTail)
		fmt.Fprintf(&sb, "%s\n", tview.Escape(strings.TrimRight(prevLogs[n], "\n")))
	}

	return sb.String()
}
