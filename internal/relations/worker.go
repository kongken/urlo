package relations

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// Start runs the refresh queue and an immediate, then periodic, full repair.
// It is safe to call repeatedly. New does not start workers on its own.
func (s *Store) Start(parent context.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed.Load() || s.cancel != nil {
		return
	}
	ctx, cancel := context.WithCancel(parent)
	s.cancel = cancel
	s.wg.Go(func() {
		for {
			select {
			case <-ctx.Done():
				return
			case code := <-s.queue:
				if err := s.refresh(ctx, code); err != nil && ctx.Err() == nil {
					s.log.Warn("owner index refresh failed; periodic repair will retry", "code", code, "error", err)
				}
			}
		}
	})
	s.wg.Go(func() {
		tick := time.Tick(s.interval)
		for {
			if err := s.Reconcile(ctx); err != nil && ctx.Err() == nil {
				s.log.Warn("owner index reconciliation failed", "error", err, "stats", s.Stats())
			} else if ctx.Err() == nil {
				s.log.Info("owner index reconciliation complete", "stats", s.Stats())
			}
			select {
			case <-ctx.Done():
				return
			case <-tick:
			}
		}
	})
}

// Close cancels database/source operations before waiting for workers. The queue
// is deliberately not drained: reconciliation after restart repairs lost work.
func (s *Store) Close() {
	s.mu.Lock()
	s.closed.Store(true)
	if s.cancel != nil {
		s.cancel()
	}
	s.mu.Unlock()
	s.wg.Wait()
}

func (s *Store) refresh(parent context.Context, code string) error {
	ctx, cancel := context.WithTimeout(parent, s.timeout)
	defer cancel()
	if err := s.index.Refresh(ctx, code, s.Store); err != nil {
		s.failed.Add(1)
		return err
	}
	s.refreshed.Add(1)
	return nil
}

// Reconcile repairs missing refs from primary objects, then checks index refs for
// deletions/reuse. It never bulk-deletes rows based on a non-atomic source scan.
func (s *Store) Reconcile(ctx context.Context) (err error) {
	if !s.reconcileMu.TryLock() {
		return errors.New("relations: reconciliation already running")
	}
	defer s.reconcileMu.Unlock()
	s.runs.Add(1)
	defer func() {
		if err != nil {
			s.runFailures.Add(1)
		}
	}()
	const batchSize = 200
	token := ""
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		scanCtx, cancel := context.WithTimeout(ctx, s.timeout)
		codes, next, err := s.scanner.ScanCodes(scanCtx, token, batchSize)
		cancel()
		if err != nil {
			return fmt.Errorf("scan primary records: %w", err)
		}
		for _, code := range codes {
			if err := s.refresh(ctx, code); err != nil {
				return fmt.Errorf("refresh primary record %s: %w", code, err)
			}
		}
		if next == "" {
			break
		}
		if next == token {
			return errors.New("relations: source scan did not advance")
		}
		token = next
	}
	after := ""
	for {
		scanCtx, cancel := context.WithTimeout(ctx, s.timeout)
		codes, err := s.index.ScanCodes(scanCtx, after, batchSize)
		cancel()
		if err != nil {
			return fmt.Errorf("scan owner index: %w", err)
		}
		for _, code := range codes {
			if code <= after {
				return errors.New("relations: index scan did not advance")
			}
			if err := s.refresh(ctx, code); err != nil {
				return fmt.Errorf("refresh indexed record %s: %w", code, err)
			}
			after = code
		}
		if len(codes) < batchSize {
			return nil
		}
	}
}
