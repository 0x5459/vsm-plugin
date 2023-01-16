package main

import (
	"context"

	objstore "github.com/ipfs-force-community/venus-objstore"

	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/pkg/objstore/filestore"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/plugin"
)

func OnInit(ctx context.Context, manifest *plugin.Manifest) error { return nil }

func Open(cfg objstore.Config) (objstore.Store, error) { // nolint: deadcode
	return filestore.Open(cfg, true)
}
