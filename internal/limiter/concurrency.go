package limiter

import (
	"context"
	"errors"
	"sync"
	"time"
)

var (
	ErrGlobalLimitExceeded = errors.New("global concurrent request limit exceeded")
	ErrKeyLimitExceeded    = errors.New("per-key concurrent request limit exceeded")
)

type keyLimiter struct {
	sem chan struct{}
}

type ConcurrencyLimiter struct {
	globalSem     chan struct{}
	perKeyLimit   int
	queueTimeout  time.Duration
	keyLimitersMu sync.Mutex
	keyLimiters   map[string]*keyLimiter
}

func NewConcurrencyLimiter(maxGlobal int, maxPerKey int, queueTimeout time.Duration) *ConcurrencyLimiter {
	if maxGlobal <= 0 {
		maxGlobal = 1024
	}
	if maxPerKey <= 0 {
		maxPerKey = 64
	}

	return &ConcurrencyLimiter{
		globalSem:    make(chan struct{}, maxGlobal),
		perKeyLimit:  maxPerKey,
		queueTimeout: queueTimeout,
		keyLimiters:  make(map[string]*keyLimiter),
	}
}

func (l *ConcurrencyLimiter) getKeyLimiter(keyID string) *keyLimiter {
	l.keyLimitersMu.Lock()
	defer l.keyLimitersMu.Unlock()

	kl, exists := l.keyLimiters[keyID]
	if !exists {
		kl = &keyLimiter{
			sem: make(chan struct{}, l.perKeyLimit),
		}
		l.keyLimiters[keyID] = kl
	}
	return kl
}

func (l *ConcurrencyLimiter) Acquire(ctx context.Context, keyID string) (func(), error) {
	// Step 1: Acquire per-key limit first
	var kl *keyLimiter
	if keyID != "" {
		kl = l.getKeyLimiter(keyID)
		select {
		case kl.sem <- struct{}{}:
			// Acquired immediately
		default:
			timeout := l.queueTimeout
			if timeout <= 0 {
				return nil, ErrKeyLimitExceeded
			}
			timer := time.NewTimer(timeout)
			defer timer.Stop()
			select {
			case kl.sem <- struct{}{}:
			case <-timer.C:
				return nil, ErrKeyLimitExceeded
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
	}

	// Step 2: Acquire global limit
	select {
	case l.globalSem <- struct{}{}:
		// Acquired immediately
	default:
		timeout := l.queueTimeout
		if timeout <= 0 {
			if kl != nil {
				<-kl.sem
			}
			return nil, ErrGlobalLimitExceeded
		}
		globalTimer := time.NewTimer(timeout)
		defer globalTimer.Stop()
		select {
		case l.globalSem <- struct{}{}:
		case <-globalTimer.C:
			if kl != nil {
				<-kl.sem
			}
			return nil, ErrGlobalLimitExceeded
		case <-ctx.Done():
			if kl != nil {
				<-kl.sem
			}
			return nil, ctx.Err()
		}
	}

	release := func() {
		<-l.globalSem
		if kl != nil {
			<-kl.sem
		}
	}

	return release, nil
}
