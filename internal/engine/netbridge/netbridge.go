// Package netbridge implements dynamic port allocation and router forwarding.
package netbridge

import (
	"context"
	"strconv"
	"sync"
	"time"

	"github.com/char2cs/asynx"

	"github.com/rabbytesoftware/quiver.core/internal/domain/netbridge"
	"github.com/rabbytesoftware/quiver.core/internal/engine/netbridge/internal/commands"
	"github.com/rabbytesoftware/quiver.core/internal/engine/netbridge/internal/ports"
	"github.com/rabbytesoftware/quiver.core/internal/engine/netbridge/internal/projections"
	"github.com/rabbytesoftware/quiver.core/internal/engine/netbridge/internal/store"
	"github.com/rabbytesoftware/quiver.core/internal/engine/netbridge/internal/strategies"
)

// maxAllocateAttempts bounds how many ports Allocate adopts into the read model
// before giving up; each adoption is one stale allocation found.
const maxAllocateAttempts = 64

const (
	// strategyWait is how long an allocation waits for router discovery that
	// has not finished before going on without forwarding.
	strategyWait = 3 * time.Second
	// strategyProbeTimeout bounds each strategy's availability probe.
	strategyProbeTimeout = 5 * time.Second
)

// Netbridge manages dynamic port allocation and best-effort router forwarding.
type Netbridge interface {
	// Allocate finds an available port matching the given protocol and preferred
	// port, forwards it through the router (best-effort), and returns the
	// assigned port number.
	//
	// If the preferred port is unavailable, Netbridge finds the next available
	// port in the ephemeral range (49152–65535). If forwarding fails, the port
	// is still allocated and returned — forwarding failure is non-fatal.
	//
	// When router forwarding is disabled, allocation is unaffected and no
	// router call is made.
	//
	// Returns ErrNoPortAvailable if no port can be found.
	// Returns ErrPortOutOfRange if the preferred port is outside 1–65535.
	Allocate(
		ctx context.Context,
		ownerKey string,
		protocol netbridge.Protocol,
		preferred int,
	) (int, error)

	// DeallocateByOwner releases all ports allocated to the given owner key.
	// Reverses router forwarding for each port before releasing, unless router
	// forwarding is disabled.
	DeallocateByOwner(
		ctx context.Context,
		ownerKey string,
	) error

	// Shutdown drains the port-allocation aggregate, blocking until every
	// in-flight command has been persisted or ctx expires. Router forwarding
	// is left in place: it is owned by the router, not by this process.
	Shutdown(
		ctx context.Context,
	) error
}

type netbridgeService struct {
	ax        asynx.Asynx[ports.PortAllocation]
	readModel store.PortStore
	stratsCh  <-chan []strategies.Strategy
	// strats is guarded by mu together with stratsReady: the discovery result
	// is read from stratsCh once it arrives, and a call that finds it late
	// proceeds without forwarding instead of waiting for it.
	strats      []strategies.Strategy
	stratsReady bool
	stratsWait  time.Duration
	// mu serializes allocation. Choosing a port reads the read model, which is
	// projected asynchronously, so two allocations in flight at once (or back to
	// back before the projection runs) would pick the same candidate and the
	// second command would be rejected.
	mu sync.Mutex
	// ownsReadModel is set when Build opened the read model itself.
	ownsReadModel bool
	portStart     int
	portEnd       int
}

// New constructs a Netbridge from already-created dependencies.
// Wires the read model projection and returns the Netbridge interface.
func newNetbridge(
	ax asynx.Asynx[ports.PortAllocation],
	readModel store.PortStore,
	stratsCh <-chan []strategies.Strategy,
	portStart int,
	portEnd int,
	project bool,
) (Netbridge, error) {
	err := subscribeProjection(ax, readModel, project)
	if err != nil {
		return nil, err
	}

	return &netbridgeService{
		ax:         ax,
		readModel:  readModel,
		stratsCh:   stratsCh,
		stratsWait: strategyWait,
		portStart:  portStart,
		portEnd:    portEnd,
	}, nil
}

func subscribeProjection(
	ax asynx.Asynx[ports.PortAllocation],
	readModel store.PortStore,
	project bool,
) error {
	if !project {
		return nil
	}
	_, err := ax.Subscribe("port.*", projections.HandlePortEvent(readModel))
	return err
}

// resolveStrategies returns the router-forwarding strategies discovery found.
// Forwarding is best-effort, so a discovery still running after stratsWait is
// not waited for: this call goes without strategies and a later one picks the
// result up. Callers hold n.mu.
func (n *netbridgeService) resolveStrategies(
	ctx context.Context,
) []strategies.Strategy {
	if n.stratsReady {
		return n.strats
	}

	wait := time.NewTimer(n.stratsWait)
	defer wait.Stop()

	select {
	case found := <-n.stratsCh:
		n.strats, n.stratsReady = found, true
		return n.strats
	case <-wait.C:
		return nil
	case <-ctx.Done():
		return nil
	}
}

// Allocate finds an available port, attempts router forwarding, and records the allocation.
func (n *netbridgeService) Allocate(
	ctx context.Context,
	ownerKey string,
	protocol netbridge.Protocol,
	preferred int,
) (int, error) {
	n.mu.Lock()
	defer n.mu.Unlock()

	for attempt := 0; attempt < maxAllocateAttempts; attempt++ {
		port, err := findAvailablePort(ctx, preferred, n.portStart, n.portEnd, protocol, n.readModel)
		if err != nil {
			return 0, err
		}
		if n.adoptIfClaimed(ctx, port) {
			continue
		}

		forwarded := false
		for _, s := range n.resolveStrategies(ctx) {
			if err := s.Forward(ctx, port, protocol); err == nil {
				forwarded = true
				break
			}
		}

		_, err = n.ax.Send(ctx, commands.AllocatePort{
			Port:      port,
			Protocol:  protocol,
			OwnerKey:  ownerKey,
			Forwarded: forwarded,
		})
		if err != nil {
			return 0, err
		}
		n.waitForProjection()

		return port, nil
	}

	return 0, ErrNoPortAvailable
}

// adoptIfClaimed reports whether the event store already holds a live
// allocation of port that the read model did not know about, and records it in
// the read model so the search moves on and DeallocateByOwner can release it.
// The read model falls behind the event store when it starts empty over
// existing events (a process restart with the in-memory store, a database
// created after allocations were made) or when a projection was lost.
func (n *netbridgeService) adoptIfClaimed(
	ctx context.Context,
	port int,
) bool {
	current, err := n.ax.Get(ctx, strconv.Itoa(port))
	if err != nil || current.Port == 0 {
		return false
	}

	return n.readModel.Save(ctx, current) == nil
}

// DeallocateByOwner releases all ports allocated to the given owner key.
func (n *netbridgeService) DeallocateByOwner(
	ctx context.Context,
	ownerKey string,
) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	allocations, err := n.readModel.FindByOwner(ctx, ownerKey)
	if err != nil {
		return err
	}

	for _, alloc := range allocations {
		for _, s := range n.resolveStrategies(ctx) {
			_ = s.Reverse(ctx, alloc.Port, alloc.Protocol)
		}

		_, sendErr := n.ax.Send(ctx, commands.DeallocatePort{Port: alloc.Port})
		if sendErr != nil {
			return sendErr
		}
	}
	n.waitForProjection()

	return nil
}

// Shutdown drains the port-allocation aggregate, then closes the read model if
// Build opened it: the drain is what finishes the projections that write to it.
func (n *netbridgeService) Shutdown(
	ctx context.Context,
) error {
	err := n.ax.Shutdown(ctx)
	if n.ownsReadModel {
		if closeErr := n.readModel.Close(); err == nil {
			err = closeErr
		}
	}

	return err
}

func (n *netbridgeService) waitForProjection() {
	n.ax.WaitPublish()
}
