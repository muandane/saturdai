// Package defaults includes global cluster defaults from ConfigMap (LLD-120).
package defaults

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	keyCPURequest    = "cpuRequest"
	keyCPULimit      = "cpuLimit"
	keyMemoryRequest = "memoryRequest"
	keyMemoryLimit   = "memoryLimit"
)

// GlobalResourceDefaults holds validated quantities from the global defaults ConfigMap.
type GlobalResourceDefaults struct {
	CPURequest    resource.Quantity
	CPULimit      resource.Quantity
	MemoryRequest resource.Quantity
	MemoryLimit   resource.Quantity
}

// GlobalDefaultsStore exposes the last-known-good global defaults snapshot.
type GlobalDefaultsStore interface {
	// Snapshot returns nil if no valid ConfigMap has been loaded yet.
	Snapshot() *GlobalResourceDefaults
}

// GlobalDefaultsLoader periodically reloads a ConfigMap into an atomic snapshot (LLD-120).
type GlobalDefaultsLoader struct {
	client    client.Client
	namespace string
	name      string
	interval  time.Duration

	loaded atomic.Pointer[GlobalResourceDefaults]
}

// NewGlobalDefaultsLoader returns a loader. If namespace or name is empty, Reload is a no-op and Snapshot stays nil.
func NewGlobalDefaultsLoader(c client.Client, namespace, name string, interval time.Duration) *GlobalDefaultsLoader {
	if interval < time.Second {
		interval = time.Minute
	}
	return &GlobalDefaultsLoader{
		client:    c,
		namespace: namespace,
		name:      name,
		interval:  interval,
	}
}

// NeedLeaderElection implements manager.LeaderElectionRunnable as false so all replicas refresh defaults.
func (l *GlobalDefaultsLoader) NeedLeaderElection() bool {
	return false
}

// Start runs until ctx is cancelled.
func (l *GlobalDefaultsLoader) Start(ctx context.Context) error {
	if l.namespace == "" || l.name == "" {
		log.FromContext(ctx).Info("global defaults ConfigMap disabled (empty namespace or name)")
		return nil
	}
	logger := log.FromContext(ctx).WithValues("component", "global-defaults-loader")
	ctx = log.IntoContext(ctx, logger)
	t := time.NewTicker(l.interval)
	defer t.Stop()
	l.reload(ctx)
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-t.C:
			l.reload(ctx)
		}
	}
}

func (l *GlobalDefaultsLoader) reload(ctx context.Context) {
	logger := log.FromContext(ctx)
	cm := &corev1.ConfigMap{}
	err := l.client.Get(ctx, types.NamespacedName{Namespace: l.namespace, Name: l.name}, cm)
	if err != nil {
		if client.IgnoreNotFound(err) == nil {
			logger.Info("global defaults ConfigMap not found; keeping last good snapshot")
			return
		}
		logger.Error(err, "failed to get global defaults ConfigMap")
		return
	}
	parsed, err := ParseGlobalDefaultsConfigMap(cm.Data)
	if err != nil {
		logger.Error(err, "invalid global defaults ConfigMap; keeping last good snapshot")
		return
	}
	l.loaded.Store(parsed)
	logger.Info("loaded global defaults ConfigMap")
}

// Snapshot implements GlobalDefaultsStore.
func (l *GlobalDefaultsLoader) Snapshot() *GlobalResourceDefaults {
	return l.loaded.Load()
}

// ParseGlobalDefaultsConfigMap validates required keys and parses quantities (LLD-120).
func ParseGlobalDefaultsConfigMap(data map[string]string) (*GlobalResourceDefaults, error) {
	if data == nil {
		return nil, errors.New("configmap data is nil")
	}
	var missing []string
	for _, k := range []string{keyCPURequest, keyCPULimit, keyMemoryRequest, keyMemoryLimit} {
		if data[k] == "" {
			missing = append(missing, k)
		}
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("missing required keys: %v", missing)
	}
	cpuReq, err := resource.ParseQuantity(data[keyCPURequest])
	if err != nil {
		return nil, fmt.Errorf("cpuRequest: %w", err)
	}
	cpuLim, err := resource.ParseQuantity(data[keyCPULimit])
	if err != nil {
		return nil, fmt.Errorf("cpuLimit: %w", err)
	}
	memReq, err := resource.ParseQuantity(data[keyMemoryRequest])
	if err != nil {
		return nil, fmt.Errorf("memoryRequest: %w", err)
	}
	memLim, err := resource.ParseQuantity(data[keyMemoryLimit])
	if err != nil {
		return nil, fmt.Errorf("memoryLimit: %w", err)
	}
	return &GlobalResourceDefaults{
		CPURequest:    cpuReq,
		CPULimit:      cpuLim,
		MemoryRequest: memReq,
		MemoryLimit:   memLim,
	}, nil
}
