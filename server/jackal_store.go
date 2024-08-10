package server

import (
	"context"
	"fmt"
	"github.com/desmos-labs/cosmos-go-wallet/wallet"
	"optijack/jackal/uploader"
)

var _ KVStore = &JackalStore{}

type JackalStore struct {
	q     *uploader.Queue
	w     *wallet.Wallet
	files map[string][]byte
}

func NewJackalStore(w *wallet.Wallet) *JackalStore {
	q := uploader.NewQueue(w)
	j := JackalStore{
		q:     q,
		w:     w,
		files: make(map[string][]byte),
	}
	q.Listen()
	return &j
}

func (j *JackalStore) Get(ctx context.Context, key []byte) ([]byte, error) {
	m := j.files[string(key)]
	if m == nil {
		return nil, fmt.Errorf("file was never saved to da layer")
	}
	return uploader.DownloadFile(ctx, m, j.w)
}

func (j *JackalStore) Put(ctx context.Context, key []byte, value []byte) error {
	_, root, err := uploader.PostFile(ctx, j.q, value, j.w)
	if err != nil {
		return fmt.Errorf("failed to upload block | %w", err)
	}

	j.files[string(key)] = root

	return nil
}
