package nomad

import (
	"context"
	"maps"
	"math"
	"sync"

	"github.com/hashicorp/nomad/api"
)

type readsKey struct{}

// EveryIndex is the read index of a topic an answer shows nothing of: no
// change of it is newer than the answer.
const EveryIndex uint64 = math.MaxUint64

// Reads keeps the index of the reads made under one context, by topic.
// X-Nomad-Index is the newest change the answer includes: the index of the
// table for a list, the ModifyIndex of the object for a read of one. Every
// change of what was read at or below it is in the answer.
type Reads struct {
	mu sync.Mutex
	at map[string]uint64
}

// WithReads returns a context under which the reads note their index, and the
// holder they note it in.
func WithReads(ctx context.Context) (context.Context, *Reads) {
	reads := &Reads{}

	return context.WithValue(ctx, readsKey{}, reads), reads
}

// Indexes are the indexes noted so far, nil when there are none.
func (r *Reads) Indexes() map[string]uint64 {
	r.mu.Lock()
	defer r.mu.Unlock()

	if len(r.at) == 0 {
		return nil
	}

	return maps.Clone(r.at)
}

// NoteRead notes that a read of the topics answered at index. A topic read
// twice keeps the lowest index. It does nothing without a holder in ctx or
// without an index.
func NoteRead(ctx context.Context, index uint64, topics ...string) {
	reads, ok := ctx.Value(readsKey{}).(*Reads)
	if !ok || index == 0 {
		return
	}

	reads.mu.Lock()
	defer reads.mu.Unlock()

	if reads.at == nil {
		reads.at = map[string]uint64{}
	}

	for _, topic := range topics {
		if old, ok := reads.at[topic]; !ok || index < old {
			reads.at[topic] = index
		}
	}
}

// readAt notes the index of a read that succeeded.
func readAt(ctx context.Context, meta *api.QueryMeta, topics ...string) {
	if meta != nil {
		NoteRead(ctx, meta.LastIndex, topics...)
	}
}
