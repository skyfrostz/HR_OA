package router

import (
	"sync"
	"time"

	"golang.org/x/time/rate"
)

const maxLoginPeers = 512

type loginPeer struct {
	limit *rate.Limiter
	seen  time.Time
}

type loginLimiter struct {
	mu        sync.Mutex
	peers     map[string]loginPeer
	global    *rate.Limiter
	nextSweep time.Time
}

func newLoginLimiter() *loginLimiter {
	return &loginLimiter{
		peers:  make(map[string]loginPeer),
		global: rate.NewLimiter(rate.Every(time.Second), 40),
	}
}

func (l *loginLimiter) allow(ip string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if !now.Before(l.nextSweep) {
		for address, peer := range l.peers {
			if now.Sub(peer.seen) > 5*time.Minute {
				delete(l.peers, address)
			}
		}
		l.nextSweep = now.Add(time.Minute)
	}

	peer, exists := l.peers[ip]
	if !exists {
		if len(l.peers) >= maxLoginPeers {
			return false
		}
		peer.limit = rate.NewLimiter(rate.Every(6*time.Second), 20)
	}
	peer.seen = now
	l.peers[ip] = peer
	return peer.limit.AllowN(now, 1) && l.global.AllowN(now, 1)
}
