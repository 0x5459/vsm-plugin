package main

import (
	"context"

	objstore "github.com/ipfs-force-community/venus-objstore"

	plugin "github.com/0x5459/vsm-plugin"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/pkg/objstore/filestore"
)

func OnInit(ctx context.Context, manifest *plugin.Manifest) error { return nil }

func Open(cfg objstore.Config) (objstore.Store, error) { // nolint: deadcode
	return filestore.Open(cfg, true)
}
