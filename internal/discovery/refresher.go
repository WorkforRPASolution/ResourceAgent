package discovery

import (
	"context"
	"math/rand"
	"sync"
	"time"

	"resourceagent/internal/logger"
)

// Closeable is the minimal interface for a transport that can be closed.
type Closeable interface {
	Close() error
}

// RefresherConfig holds configuration for the address Refresher.
type RefresherConfig struct {
	Interval time.Duration
}

// Refresher periodically checks ServiceDiscovery for address changes
// and swaps the sender transport when the address changes.
type Refresher struct {
	cfg         RefresherConfig
	currentAddr string

	// Injectable functions for testability
	fetchAddr        func(ctx context.Context) (string, error)
	transportFactory func(addr string) (Closeable, error)
	swapTransport    func(newTransport Closeable) (Closeable, error)
	closeOld         func(old Closeable)

	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewRefresher creates a new Refresher with the given configuration.
func NewRefresher(cfg RefresherConfig) *Refresher {
	return &Refresher{
		cfg: cfg,
		closeOld: func(old Closeable) {
			old.Close()
		},
	}
}

// SetFetchAddr sets the function that fetches the current address from ServiceDiscovery.
func (r *Refresher) SetFetchAddr(fn func(ctx context.Context) (string, error)) {
	r.fetchAddr = fn
}

// SetTransportFactory sets the function that creates a new transport from an address.
func (r *Refresher) SetTransportFactory(fn func(addr string) (Closeable, error)) {
	r.transportFactory = fn
}

// SetSwapTransport sets the function that swaps the transport on the sender.
func (r *Refresher) SetSwapTransport(fn func(newTransport Closeable) (Closeable, error)) {
	r.swapTransport = fn
}

// Start begins periodic address checking. If interval <= 0, no goroutine is started.
func (r *Refresher) Start(ctx context.Context, initialAddr string) {
	r.currentAddr = initialAddr

	if r.cfg.Interval <= 0 {
		return
	}

	refreshCtx, cancel := context.WithCancel(ctx)
	r.cancel = cancel

	r.wg.Add(1)
	go r.loop(refreshCtx)
}

// Stop cancels the background goroutine and waits for it to finish.
func (r *Refresher) Stop() {
	if r.cancel != nil {
		r.cancel()
	}
	r.wg.Wait()
}

func (r *Refresher) loop(ctx context.Context) {
	defer r.wg.Done()
	log := logger.WithComponent("address-refresher")

	// Jitter on first tick: rand(0, interval) to prevent thundering herd
	jitter := time.Duration(rand.Int63n(int64(r.cfg.Interval)))
	log.Info().
		Dur("interval", r.cfg.Interval).
		Dur("initial_jitter", jitter).
		Msg("Address refresher started")

	select {
	case <-ctx.Done():
		return
	case <-time.After(r.cfg.Interval + jitter):
		r.refreshOnce(ctx)
	}

	ticker := time.NewTicker(r.cfg.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.refreshOnce(ctx)
		}
	}
}

func (r *Refresher) refreshOnce(ctx context.Context) {
	log := logger.WithComponent("address-refresher")

	newAddr, err := r.fetchAddr(ctx)
	if err != nil {
		log.Warn().Err(err).Msg("Failed to fetch address from ServiceDiscovery")
		return
	}

	if newAddr == r.currentAddr {
		return
	}

	log.Info().
		Str("old_addr", r.currentAddr).
		Str("new_addr", newAddr).
		Msg("Server address changed, swapping transport")

	newTransport, err := r.transportFactory(newAddr)
	if err != nil {
		log.Warn().Err(err).
			Str("new_addr", newAddr).
			Msg("Failed to create new transport")
		return
	}

	oldTransport, err := r.swapTransport(newTransport)
	if err != nil {
		log.Warn().Err(err).Msg("Failed to swap transport")
		newTransport.Close()
		return
	}

	oldAddr := r.currentAddr
	r.currentAddr = newAddr

	go r.closeOld(oldTransport)

	log.Info().
		Str("old_addr", oldAddr).
		Str("new_addr", newAddr).
		Msg("Server address updated successfully")
}
