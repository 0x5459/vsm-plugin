package internal

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/fatih/color"
	cbor "github.com/ipfs/go-ipld-cbor"
	"github.com/urfave/cli/v2"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-bitfield"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"

	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/core"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/modules/policy"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/pkg/messager"

	"github.com/filecoin-project/venus/app/submodule/chain"
	"github.com/filecoin-project/venus/venus-shared/actors"
	"github.com/filecoin-project/venus/venus-shared/actors/adt"
	"github.com/filecoin-project/venus/venus-shared/actors/builtin/miner"
	specpolicy "github.com/filecoin-project/venus/venus-shared/actors/policy"
	"github.com/filecoin-project/venus/venus-shared/types"
)

var utilSealerCmd = &cli.Command{
	Name:  "sealer",
	Flags: []cli.Flag{},
	Subcommands: []*cli.Command{
		utilSealerSectorsCmd,
		utilSealerProvingCmd,
		utilSealerActorCmd,
		utilSealerSnapCmd,
	},
}

var utilSealerSectorsCmd = &cli.Command{
	Name: "sectors",
	Subcommands: []*cli.Command{
		utilSealerSectorsWorkerStatesCmd,
		utilSealerSectorsAbortCmd,
		utilSealerSectorsListCmd,
		utilSealerSectorsRestoreCmd,
		utilSealerSectorsCheckExpireCmd,
		utilSealerSectorsExpiredCmd,
		utilSealerSectorsRenewCmd,
		utilSealerSectorsExtendCmd,
		utilSealerSectorsTerminateCmd,
		utilSealerSectorsRemoveCmd,
	},
}

var utilSealerSectorsWorkerStatesCmd = &cli.Command{
	Name: "worker-states",
	Action: func(cctx *cli.Context) error {
		cli, gctx, stop, err := extractAPI(cctx)
		if err != nil {
			return err
		}

		defer stop()

		states, err := cli.Sealer.ListSectors(gctx, core.WorkerOnline)
		if err != nil {
			return err
		}

		fmt.Fprintf(os.Stdout, "Sectors(%d):\n", len(states))
		for _, state := range states {
			fmt.Fprintf(os.Stdout, "m-%d-s-%d:\n", state.ID.Miner, state.ID.Number)
			if state.LatestState == nil {
				fmt.Fprintln(os.Stdout, "NULL")
				continue
			}

			fmt.Fprintln(os.Stdout, "\tWorker:")
			fmt.Fprintf(os.Stdout, "\t\tInstance: %s\n", state.LatestState.Worker.Instance)
			fmt.Fprintf(os.Stdout, "\t\tLocation: %s\n", state.LatestState.Worker.Location)

			fmt.Fprintln(os.Stdout, "\tDeals:")
			deals := state.Deals()
			if len(deals) == 0 {
				fmt.Fprintln(os.Stdout, "\t\tNULL")
			} else {
				for _, deal := range deals {
					fmt.Fprintf(os.Stdout, "\t\tID: %d\n", deal.ID)
					fmt.Fprintf(os.Stdout, "\t\tPiece: %v\n", deal.Piece)
				}
			}

			fmt.Fprintln(os.Stdout, "\tTicket:")
			if state.Ticket != nil {
				fmt.Fprintf(os.Stdout, "\t\tHeight: %d\n", state.Ticket.Epoch)
				fmt.Fprintf(os.Stdout, "\t\tValue: %x\n", state.Ticket.Ticket)
			}

			fmt.Fprintln(os.Stdout, "\tSeed:")
			if state.Seed != nil {
				fmt.Fprintf(os.Stdout, "\t\tHeight: %d\n", state.Seed.Epoch)
				fmt.Fprintf(os.Stdout, "\t\tValue: %x\n", state.Seed.Seed)
			}

			fmt.Fprintln(os.Stdout, "\tMessageInfo:")
			if state.MessageInfo.PreCommitCid != nil {
				fmt.Fprintf(os.Stdout, "\t\tPre: %s\n", state.MessageInfo.PreCommitCid.String())
			}
			if state.MessageInfo.CommitCid != nil {
				fmt.Fprintf(os.Stdout, "\t\tProve: %s\n", state.MessageInfo.CommitCid.String())
			}

			fmt.Fprintln(os.Stdout, "\tState:")
			fmt.Fprintf(os.Stdout, "\t\tPrev: %s\n", state.LatestState.StateChange.Prev)
			fmt.Fprintf(os.Stdout, "\t\tCurrent: %s\n", state.LatestState.StateChange.Next)
			fmt.Fprintf(os.Stdout, "\t\tEvent: %s\n", state.LatestState.StateChange.Event)

			fmt.Fprintln(os.Stdout, "\tFailure:")
			if state.LatestState.Failure == nil {
				fmt.Fprintln(os.Stdout, "\t\tNULL")
			} else {
				fmt.Fprintf(os.Stdout, "\t\tLevel: %s\n", state.LatestState.Failure.Level)
				fmt.Fprintf(os.Stdout, "\t\tDesc: %s\n", state.LatestState.Failure.Desc)
			}

			fmt.Fprintln(os.Stdout, "")
		}

		return nil
	},
}

var utilSealerSectorsAbortCmd = &cli.Command{
	Name:      "abort",
	ArgsUsage: "<miner actor> <sector number>",
	Action: func(cctx *cli.Context) error {
		if count := cctx.Args().Len(); count < 2 {
			return fmt.Errorf("both miner actor id & sector number are required, only %d args provided", count)
		}

		miner, err := ShouldActor(cctx.Args().Get(0), true)
		if err != nil {
			return fmt.Errorf("invalid miner actor id: %w", err)
		}

		sectorNum, err := strconv.ParseUint(cctx.Args().Get(1), 10, 64)
		if err != nil {
			return fmt.Errorf("invalid sector number: %w", err)
		}

		cli, gctx, stop, err := extractAPI(cctx)
		if err != nil {
			return err
		}

		defer stop()

		_, err = cli.Sealer.ReportAborted(gctx, abi.SectorID{
			Miner:  miner,
			Number: abi.SectorNumber(sectorNum),
		}, "aborted via CLI")
		if err != nil {
			return fmt.Errorf("abort sector failed: %w", err)
		}

		return nil
	},
}

var utilSealerSectorsListCmd = &cli.Command{
	Name:  "list",
	Usage: "Print sector data in completed state",
	Action: func(cctx *cli.Context) error {
		cli, gctx, stop, err := extractAPI(cctx)
		if err != nil {
			return err
		}

		defer stop()

		states, err := cli.Sealer.ListSectors(gctx, core.WorkerOffline)
		if err != nil {
			return err
		}

		fmt.Fprintf(os.Stdout, "Sectors(%d):\n", len(states))
		for _, state := range states {
			fmt.Fprintf(os.Stdout, "m-%d-s-%d:\n", state.ID.Miner, state.ID.Number)
			if state.LatestState == nil {
				fmt.Fprintln(os.Stdout, "NULL")
				continue
			}

			fmt.Fprintln(os.Stdout, "\tDeals:")
			deals := state.Deals()
			if len(deals) == 0 {
				fmt.Fprintln(os.Stdout, "\t\tNULL")
			} else {
				for _, deal := range deals {
					fmt.Fprintf(os.Stdout, "\t\tID: %d\n", deal.ID)
					fmt.Fprintf(os.Stdout, "\t\tPiece: %v\n", deal.Piece)
				}
			}

			fmt.Fprintln(os.Stdout, "\tTicket:")
			if state.Ticket != nil {
				fmt.Fprintf(os.Stdout, "\t\tHeight: %d\n", state.Ticket.Epoch)
				fmt.Fprintf(os.Stdout, "\t\tValue: %x\n", state.Ticket.Ticket)
			} else {
				fmt.Fprintln(os.Stdout, "\t\tNULL")
			}

			fmt.Fprintln(os.Stdout, "\tSeed:")
			if state.Seed != nil {
				fmt.Fprintf(os.Stdout, "\t\tHeight: %d\n", state.Seed.Epoch)
				fmt.Fprintf(os.Stdout, "\t\tValue: %x\n", state.Seed.Seed)
			} else {
				fmt.Fprintln(os.Stdout, "\t\tNULL")
			}

			fmt.Fprintln(os.Stdout, "\tMessageInfo:")
			if state.MessageInfo.PreCommitCid != nil {
				fmt.Fprintf(os.Stdout, "\t\tPre: %s\n", state.MessageInfo.PreCommitCid.String())
			} else {
				fmt.Fprintln(os.Stdout, "\t\tPre: NULL")
			}
			if state.MessageInfo.CommitCid != nil {
				fmt.Fprintf(os.Stdout, "\t\tProve: %s\n", state.MessageInfo.CommitCid.String())
			} else {
				fmt.Fprintln(os.Stdout, "\t\tProve: NULL")
			}
			fmt.Fprintf(os.Stdout, "\t\tNeedSeed: %v\n", state.MessageInfo.NeedSend)

			fmt.Fprintln(os.Stdout, "\tTerminateInfo:")
			if state.TerminateInfo.TerminateCid != nil {
				fmt.Fprintf(os.Stdout, "\t\tUid: %s\n", state.TerminateInfo.TerminateCid.String())
				fmt.Fprintf(os.Stdout, "\t\tHeight: %v\n", state.TerminateInfo.TerminatedAt)
				fmt.Fprintf(os.Stdout, "\t\tAddedHeight: %v\n", state.TerminateInfo.AddedHeight)
			} else {
				fmt.Fprintln(os.Stdout, "\t\tNULL")
			}

			fmt.Fprintln(os.Stdout, "\tState:")
			fmt.Fprintf(os.Stdout, "\t\tPrev: %s\n", state.LatestState.StateChange.Prev)
			fmt.Fprintf(os.Stdout, "\t\tCurrent: %s\n", state.LatestState.StateChange.Next)
			fmt.Fprintf(os.Stdout, "\t\tEvent: %s\n", state.LatestState.StateChange.Event)

			fmt.Fprintf(os.Stdout, "\tFinalized: %v\n", state.Finalized)
			fmt.Fprintf(os.Stdout, "\tRemoved: %v\n", state.Removed)

			fmt.Fprintln(os.Stdout, "")
		}

		return nil
	},
}

var utilSealerSectorsRestoreCmd = &cli.Command{
	Name:  "restore",
	Usage: "restore a sector state that may already finalized or aborted",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:   "force",
			Hidden: true,
			Value:  false,
		},
	},
	ArgsUsage: "<miner actor id> <sector number>",
	Action: func(cctx *cli.Context) error {
		if count := cctx.Args().Len(); count < 2 {
			return fmt.Errorf("both miner actor id & sector number are required, only %d args provided", count)
		}

		miner, err := ShouldActor(cctx.Args().Get(0), true)
		if err != nil {
			return fmt.Errorf("invalid miner actor id: %w", err)
		}

		sectorNum, err := strconv.ParseUint(cctx.Args().Get(1), 10, 64)
		if err != nil {
			return fmt.Errorf("invalid sector number: %w", err)
		}

		cli, gctx, stop, err := extractAPI(cctx)
		if err != nil {
			return err
		}

		defer stop()

		_, err = cli.Sealer.RestoreSector(gctx, abi.SectorID{
			Miner:  miner,
			Number: abi.SectorNumber(sectorNum),
		}, cctx.Bool("force"))
		if err != nil {
			return fmt.Errorf("restore sector failed: %w", err)
		}

		return nil
	},
}

var utilSealerSectorsCheckExpireCmd = &cli.Command{
	Name:  "check-expire",
	Usage: "Inspect expiring sectors",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name: "miner",
		},
		&cli.Int64Flag{
			Name:  "cutoff",
			Usage: "skip sectors whose current expiration is more than <cutoff> epochs from now, defaults to 60 days",
			Value: 172800,
		},
	},
	Action: func(cctx *cli.Context) error {
		maddr, err := ShouldAddress(cctx.String("miner"), true, true)
		if err != nil {
			return err
		}

		api, ctx, stop, err := extractAPI(cctx)
		if err != nil {
			return err
		}
		defer stop()

		head, err := api.Chain.ChainHead(ctx)
		if err != nil {
			return err
		}
		currEpoch := head.Height()

		nv, err := api.Chain.StateNetworkVersion(ctx, types.EmptyTSK)
		if err != nil {
			return err
		}

		sectors, err := api.Chain.StateMinerActiveSectors(ctx, maddr, types.EmptyTSK)
		if err != nil {
			return err
		}

		n := 0
		for _, s := range sectors {
			if s.Expiration-currEpoch <= abi.ChainEpoch(cctx.Int64("cutoff")) {
				sectors[n] = s
				n++
			}
		}
		sectors = sectors[:n]

		sort.Slice(sectors, func(i, j int) bool {
			if sectors[i].Expiration == sectors[j].Expiration {
				return sectors[i].SectorNumber < sectors[j].SectorNumber
			}
			return sectors[i].Expiration < sectors[j].Expiration
		})

		blockDelaySecs := policy.NetParams.Network.BlockDelay
		fmt.Fprintf(os.Stdout, "Sectors(%d):\n", len(sectors))
		for _, sector := range sectors {
			MaxExpiration := sector.Activation + specpolicy.GetSectorMaxLifetime(sector.SealProof, nv)
			MaxExtendNow := currEpoch + specpolicy.GetMaxSectorExpirationExtension()

			if MaxExtendNow > MaxExpiration {
				MaxExtendNow = MaxExpiration
			}

			fmt.Fprintf(os.Stdout, "\tID: %d\n", sector.SectorNumber)
			fmt.Fprintf(os.Stdout, "\tSealProof: %d\n", sector.SealProof)
			fmt.Fprintf(os.Stdout, "\tInitialPledge: %v\n", types.FIL(sector.InitialPledge).Short())
			fmt.Fprintf(os.Stdout, "\tActivation: %s\n", EpochTime(currEpoch, sector.Activation, blockDelaySecs))
			fmt.Fprintf(os.Stdout, "\tExpiration: %s\n", EpochTime(currEpoch, sector.Expiration, blockDelaySecs))
			fmt.Fprintf(os.Stdout, "\tMaxExpiration: %s\n", EpochTime(currEpoch, MaxExpiration, blockDelaySecs))
			fmt.Fprintf(os.Stdout, "\tMaxExtendNow: %s\n", EpochTime(currEpoch, MaxExtendNow, blockDelaySecs))

			fmt.Fprintln(os.Stdout, "")
		}

		return nil
	},
}

var utilSealerSectorsExpiredCmd = &cli.Command{
	Name:  "expired",
	Usage: "Get or cleanup expired sectors",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name: "miner",
		},

		&cli.BoolFlag{
			Name:  "show-removed",
			Usage: "show removed sectors",
		},
		&cli.BoolFlag{
			Name:  "remove-expired",
			Usage: "remove expired sectors",
		},

		&cli.Int64Flag{
			Name:   "confirm-remove-count",
			Hidden: true,
		},
		&cli.Int64Flag{
			Name:        "expired-epoch",
			Usage:       "epoch at which to check sector expirations",
			DefaultText: "WinningPoSt lookback epoch",
		},
	},
	Action: func(cctx *cli.Context) error {
		maddr, err := ShouldAddress(cctx.String("miner"), true, true)
		if err != nil {
			return err
		}

		extAPI, ctx, stop, err := extractAPI(cctx)
		if err != nil {
			return err
		}
		defer stop()

		head, err := extAPI.Chain.ChainHead(ctx)
		if err != nil {
			return fmt.Errorf("getting chain head: %w", err)
		}

		lbEpoch := abi.ChainEpoch(cctx.Int64("expired-epoch"))
		if !cctx.IsSet("expired-epoch") {
			nv, err := extAPI.Chain.StateNetworkVersion(ctx, head.Key())
			if err != nil {
				return fmt.Errorf("getting network version: %w", err)
			}

			lbEpoch = head.Height() - specpolicy.GetWinningPoStSectorSetLookback(nv)
			if lbEpoch < 0 {
				return fmt.Errorf("too early to terminate sectors")
			}
		}

		if cctx.IsSet("confirm-remove-count") && !cctx.IsSet("expired-epoch") {
			return fmt.Errorf("--expired-epoch must be specified with --confirm-remove-count")
		}

		lbts, err := extAPI.Chain.ChainGetTipSetByHeight(ctx, lbEpoch, head.Key())
		if err != nil {
			return fmt.Errorf("getting lookback tipset: %w", err)
		}

		// toCheck is a working bitfield which will only contain terminated sectors
		toCheck := bitfield.New()
		toCheckSectors := make(map[abi.SectorNumber]*core.SectorState)
		{
			sectors, err := extAPI.Sealer.ListSectors(ctx, core.WorkerOffline)
			if err != nil {
				return fmt.Errorf("getting sector list: %w", err)
			}

			for _, sector := range sectors {
				toCheck.Set(uint64(sector.ID.Number))
				toCheckSectors[sector.ID.Number] = sector
			}
		}

		mact, err := extAPI.Chain.StateGetActor(ctx, maddr, lbts.Key())
		if err != nil {
			return err
		}

		store := adt.WrapStore(ctx, cbor.NewCborStore(chain.NewAPIBlockstore(extAPI.Chain)))
		mas, err := miner.Load(store, mact)
		if err != nil {
			return err
		}

		alloc, err := mas.GetAllocatedSectors()
		if err != nil {
			return fmt.Errorf("getting allocated sectors: %w", err)
		}

		// only allocated sectors can be expired,
		toCheck, err = bitfield.IntersectBitField(toCheck, *alloc)
		if err != nil {
			return fmt.Errorf("intersecting bitfields: %w", err)
		}

		if err := mas.ForEachDeadline(func(dlIdx uint64, dl miner.Deadline) error {
			return dl.ForEachPartition(func(partIdx uint64, part miner.Partition) error {
				live, err := part.LiveSectors()
				if err != nil {
					return err
				}

				toCheck, err = bitfield.SubtractBitField(toCheck, live)
				if err != nil {
					return err
				}

				unproven, err := part.UnprovenSectors()
				if err != nil {
					return err
				}

				toCheck, err = bitfield.SubtractBitField(toCheck, unproven)

				return err
			})
		}); err != nil {
			return err
		}

		err = mas.ForEachPrecommittedSector(func(pci miner.SectorPreCommitOnChainInfo) error {
			toCheck.Unset(uint64(pci.Info.SectorNumber))
			return nil
		})
		if err != nil {
			return err
		}

		if cctx.Bool("remove-expired") {
			color.Red("Removing sectors:\n")
		}

		// toCheck now only contains sectors which either failed to precommit or are expired/terminated
		fmt.Printf("SectorID\n")

		var toRemove []abi.SectorNumber
		err = toCheck.ForEach(func(u uint64) error {
			sn := abi.SectorNumber(u)

			if sector, ok := toCheckSectors[sn]; ok {
				if sector.Removed {
					if cctx.IsSet("confirm-remove-count") || !cctx.Bool("show-removed") {
						return nil
					}
				} else { // not removed
					toRemove = append(toRemove, sn)
				}

				fmt.Printf("%d\n", sn)
			}

			return nil
		})
		if err != nil {
			return err
		}

		if cctx.Bool("remove-expired") {
			if !cctx.IsSet("confirm-remove-count") {
				fmt.Println()
				fmt.Println(color.YellowString("All"), color.GreenString("%d", len(toRemove)), color.YellowString("sectors listed above will be removed\n"))
				fmt.Println(color.YellowString("To confirm removal of the above sectors, including\n all related sealed and unsealed data, run:\n"))
				fmt.Println(color.RedString("venus-sealer sectors expired --remove-expired --confirm-remove-count=%d --expired-epoch=%d\n", len(toRemove), lbts.Height()))
				fmt.Println(color.YellowString("WARNING: This operation is irreversible"))
				return nil
			}

			fmt.Println()

			if int64(len(toRemove)) != cctx.Int64("confirm-remove-count") {
				return fmt.Errorf("value of confirm-remove-count doesn't match the number of sectors which can be removed (%d)", len(toRemove))
			}

			actor, _ := address.IDFromAddress(maddr)
			for _, number := range toRemove {
				fmt.Printf("Removing sector\t%s:\t", color.YellowString("%d", number))

				err = extAPI.Sealer.RemoveSector(ctx, abi.SectorID{Miner: abi.ActorID(actor), Number: number})
				if err != nil {
					color.Red("ERROR: %s\n", err.Error())
				} else {
					color.Green("OK\n")
				}
			}
		}

		return nil
	},
}

func getSectorsFromFile(filePath string) ([]uint64, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}

	scanner := bufio.NewScanner(file)
	sectors := make([]uint64, 0)

	for scanner.Scan() {
		line := scanner.Text()

		id, err := strconv.ParseUint(line, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("could not parse %s as sector id: %s", line, err)
		}

		sectors = append(sectors, id)
	}

	if err = file.Close(); err != nil {
		return nil, err
	}

	return sectors, nil
}

type PseudoExpirationExtension struct {
	Deadline      uint64
	Partition     uint64
	Sectors       string
	NewExpiration abi.ChainEpoch
}

type PseudoExtendSectorExpirationParams struct {
	Extensions []PseudoExpirationExtension
}

// ArrayToString Example: {1,3,4,5,8,9} -> "1,3-5,8-9"
func ArrayToString(array []uint64) string {
	sort.Slice(array, func(i, j int) bool {
		return array[i] < array[j]
	})

	var sarray []string
	s := ""

	for i, elm := range array {
		if i == 0 {
			s = strconv.FormatUint(elm, 10)
			continue
		}
		if elm == array[i-1] {
			continue // filter out duplicates
		} else if elm == array[i-1]+1 {
			s = strings.Split(s, "-")[0] + "-" + strconv.FormatUint(elm, 10)
		} else {
			sarray = append(sarray, s)
			s = strconv.FormatUint(elm, 10)
		}
	}

	if s != "" {
		sarray = append(sarray, s)
	}

	return strings.Join(sarray, ",")
}

func NewPseudoExtendParams(p *core.ExtendSectorExpirationParams) (*PseudoExtendSectorExpirationParams, error) {
	res := PseudoExtendSectorExpirationParams{}
	for _, ext := range p.Extensions {
		scount, err := ext.Sectors.Count()
		if err != nil {
			return nil, err
		}

		sectors, err := ext.Sectors.All(scount)
		if err != nil {
			return nil, err
		}

		res.Extensions = append(res.Extensions, PseudoExpirationExtension{
			Deadline:      ext.Deadline,
			Partition:     ext.Partition,
			Sectors:       ArrayToString(sectors),
			NewExpiration: ext.NewExpiration,
		})
	}
	return &res, nil
}

var utilSealerSectorsRenewCmd = &cli.Command{
	Name:  "renew",
	Usage: "Renew expiring sectors while not exceeding each sector's max life",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name: "miner",
		},
		&cli.Int64Flag{
			Name:  "from",
			Usage: "only consider sectors whose current expiration epoch is in the range of [from, to], <from> defaults to: now + 120 (1 hour)",
		},
		&cli.Int64Flag{
			Name:  "to",
			Usage: "only consider sectors whose current expiration epoch is in the range of [from, to], <to> defaults to: now + 92160 (32 days)",
		},
		&cli.StringFlag{
			Name:  "sector-file",
			Usage: "provide a file containing one sector number in each line, ignoring above selecting criteria",
		},
		&cli.StringFlag{
			Name:  "exclude",
			Usage: "optionally provide a file containing excluding sectors",
		},
		&cli.Int64Flag{
			Name:  "extension",
			Usage: "try to extend selected sectors by this number of epochs, defaults to 540 days",
			Value: 1555200,
		},
		&cli.Int64Flag{
			Name:  "new-expiration",
			Usage: "try to extend selected sectors to this epoch, ignoring extension",
		},
		&cli.Int64Flag{
			Name:  "tolerance",
			Usage: "don't try to extend sectors by fewer than this number of epochs, defaults to 7 days",
			Value: 20160,
		},
		&cli.StringFlag{
			Name:  "max-fee",
			Usage: "use up to this amount of FIL for one message. pass this flag to avoid message congestion.",
			Value: "0",
		},
		&cli.BoolFlag{
			Name:  "really-do-it",
			Usage: "pass this flag to really renew sectors, otherwise will only print out json representation of parameters",
		},
	},
	Action: func(cctx *cli.Context) error {
		mf, err := types.ParseFIL(cctx.String("max-fee"))
		if err != nil {
			return err
		}
		spec := &messager.MsgMeta{MaxFee: abi.TokenAmount(mf)}

		maddr, err := ShouldAddress(cctx.String("miner"), true, true)
		if err != nil {
			return err
		}

		fapi, ctx, stop, err := extractAPI(cctx)
		if err != nil {
			return err
		}
		defer stop()

		head, err := fapi.Chain.ChainHead(ctx)
		if err != nil {
			return err
		}
		currEpoch := head.Height()

		nv, err := fapi.Chain.StateNetworkVersion(ctx, types.EmptyTSK)
		if err != nil {
			return err
		}

		activeSet, err := fapi.Chain.StateMinerActiveSectors(ctx, maddr, types.EmptyTSK)
		if err != nil {
			return err
		}

		activeSectorsInfo := make(map[abi.SectorNumber]*miner.SectorOnChainInfo, len(activeSet))
		for _, info := range activeSet {
			activeSectorsInfo[info.SectorNumber] = info
		}

		mact, err := fapi.Chain.StateGetActor(ctx, maddr, types.EmptyTSK)
		if err != nil {
			return err
		}

		store := adt.WrapStore(ctx, cbor.NewCborStore(chain.NewAPIBlockstore(fapi.Chain)))
		mas, err := miner.Load(store, mact)
		if err != nil {
			return err
		}

		activeSectorsLocation := make(map[abi.SectorNumber]*miner.SectorLocation, len(activeSet))

		if err := mas.ForEachDeadline(func(dlIdx uint64, dl miner.Deadline) error {
			return dl.ForEachPartition(func(partIdx uint64, part miner.Partition) error {
				pas, err := part.ActiveSectors()
				if err != nil {
					return err
				}

				return pas.ForEach(func(i uint64) error {
					activeSectorsLocation[abi.SectorNumber(i)] = &miner.SectorLocation{
						Deadline:  dlIdx,
						Partition: partIdx,
					}
					return nil
				})
			})
		}); err != nil {
			return err
		}

		excludeSet := make(map[uint64]struct{})

		if cctx.IsSet("exclude") {
			excludeSectors, err := getSectorsFromFile(cctx.String("exclude"))
			if err != nil {
				return err
			}

			for _, id := range excludeSectors {
				excludeSet[id] = struct{}{}
			}
		}

		var sis []*miner.SectorOnChainInfo

		if cctx.IsSet("sector-file") {
			sectors, err := getSectorsFromFile(cctx.String("sector-file"))
			if err != nil {
				return err
			}

			for _, id := range sectors {
				if _, exclude := excludeSet[id]; exclude {
					continue
				}

				si, found := activeSectorsInfo[abi.SectorNumber(id)]
				if !found {
					return fmt.Errorf("sector %d is not active", id)
				}

				sis = append(sis, si)
			}
		} else {
			from := currEpoch + 120
			to := currEpoch + 92160

			if cctx.IsSet("from") {
				from = abi.ChainEpoch(cctx.Int64("from"))
			}

			if cctx.IsSet("to") {
				to = abi.ChainEpoch(cctx.Int64("to"))
			}

			for _, si := range activeSet {
				if si.Expiration >= from && si.Expiration <= to {
					if _, exclude := excludeSet[uint64(si.SectorNumber)]; !exclude {
						sis = append(sis, si)
					}
				}
			}
		}

		extensions := map[miner.SectorLocation]map[abi.ChainEpoch][]uint64{}

		withinTolerance := func(a, b abi.ChainEpoch) bool {
			diff := a - b
			if diff < 0 {
				diff = -diff
			}

			return diff <= abi.ChainEpoch(cctx.Int64("tolerance"))
		}

		for _, si := range sis {
			extension := abi.ChainEpoch(cctx.Int64("extension"))
			newExp := si.Expiration + extension

			if cctx.IsSet("new-expiration") {
				newExp = abi.ChainEpoch(cctx.Int64("new-expiration"))
			}

			maxExtendNow := currEpoch + specpolicy.GetMaxSectorExpirationExtension()
			if newExp > maxExtendNow {
				newExp = maxExtendNow
			}

			maxExp := si.Activation + specpolicy.GetSectorMaxLifetime(si.SealProof, nv)
			if newExp > maxExp {
				newExp = maxExp
			}

			if newExp <= si.Expiration || withinTolerance(newExp, si.Expiration) {
				continue
			}

			l, found := activeSectorsLocation[si.SectorNumber]
			if !found {
				return fmt.Errorf("location for sector %d not found", si.SectorNumber)
			}

			es, found := extensions[*l]
			if !found {
				ne := make(map[abi.ChainEpoch][]uint64)
				ne[newExp] = []uint64{uint64(si.SectorNumber)}
				extensions[*l] = ne
			} else {
				added := false
				for exp := range es {
					if withinTolerance(newExp, exp) {
						es[exp] = append(es[exp], uint64(si.SectorNumber))
						added = true
						break
					}
				}

				if !added {
					es[newExp] = []uint64{uint64(si.SectorNumber)}
				}
			}
		}

		var params []core.ExtendSectorExpirationParams

		p := core.ExtendSectorExpirationParams{}
		scount := 0

		for l, exts := range extensions {
			for newExp, numbers := range exts {
				scount += len(numbers)
				addrSectors, err := specpolicy.GetAddressedSectorsMax(nv)
				if err != nil {
					return err
				}
				declMax, err := specpolicy.GetDeclarationsMax(nv)
				if err != nil {
					return err
				}
				if scount > addrSectors || len(p.Extensions) == declMax {
					params = append(params, p)
					p = core.ExtendSectorExpirationParams{}
					scount = len(numbers)
				}

				p.Extensions = append(p.Extensions, core.ExpirationExtension{
					Deadline:      l.Deadline,
					Partition:     l.Partition,
					Sectors:       bitfield.NewFromSet(numbers),
					NewExpiration: newExp,
				})
			}
		}

		// if we have any sectors, then one last append is needed here
		if scount != 0 {
			params = append(params, p)
		}

		if len(params) == 0 {
			fmt.Println("nothing to extend")
			return nil
		}

		mi, err := fapi.Chain.StateMinerInfo(ctx, maddr, types.EmptyTSK)
		if err != nil {
			return fmt.Errorf("getting miner info: %w", err)
		}

		stotal := 0

		for i := range params {
			scount := 0
			for _, ext := range params[i].Extensions {
				count, err := ext.Sectors.Count()
				if err != nil {
					return err
				}
				scount += int(count)
			}
			fmt.Printf("Renewing %d sectors: ", scount)
			stotal += scount

			if !cctx.Bool("really-do-it") {
				pp, err := NewPseudoExtendParams(&params[i])
				if err != nil {
					return err
				}

				data, err := json.MarshalIndent(pp, "", "  ")
				if err != nil {
					return err
				}

				fmt.Println()
				fmt.Println(string(data))
				continue
			}

			sp, aerr := actors.SerializeParams(&params[i])
			if aerr != nil {
				return fmt.Errorf("serializing params: %w", err)
			}

			msg := &types.Message{
				From: mi.Worker,

				To:     maddr,
				Method: miner.Methods.ExtendSectorExpiration,
				Params: sp,

				Value: big.Zero(),
			}

			mid, err := fapi.Messager.PushMessage(ctx, msg, spec)
			if err != nil {
				return RPCCallError("PushMessageWithId", err)
			}

			fmt.Println(mid)
		}

		fmt.Printf("%d sectors renewed\n", stotal)

		return nil
	},
}

var utilSealerSectorsExtendCmd = &cli.Command{
	Name:      "extend",
	Usage:     "Extend sector expiration",
	ArgsUsage: "<sectorNumbers...>",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name: "miner",
		},
		&cli.Int64Flag{
			Name:     "new-expiration",
			Usage:    "new expiration epoch",
			Required: false,
		},
		&cli.BoolFlag{
			Name:     "v1-sectors",
			Usage:    "renews all v1 sectors up to the maximum possible lifetime",
			Required: false,
		},
		&cli.Int64Flag{
			Name:     "tolerance",
			Value:    20160,
			Usage:    "when extending v1 sectors, don't try to extend sectors by fewer than this number of epochs",
			Required: false,
		},
		&cli.Int64Flag{
			Name:     "expiration-ignore",
			Value:    120,
			Usage:    "when extending v1 sectors, skip sectors whose current expiration is less than <ignore> epochs from now",
			Required: false,
		},
		&cli.Int64Flag{
			Name:     "expiration-cutoff",
			Usage:    "when extending v1 sectors, skip sectors whose current expiration is more than <cutoff> epochs from now (infinity if unspecified)",
			Required: false,
		},
		&cli.StringFlag{},
	},
	Action: func(cctx *cli.Context) error {
		maddr, err := ShouldAddress(cctx.String("miner"), true, true)
		if err != nil {
			return err
		}

		fapi, ctx, stop, err := extractAPI(cctx)
		if err != nil {
			return err
		}
		defer stop()

		var params []core.ExtendSectorExpirationParams

		if cctx.Bool("v1-sectors") {

			head, err := fapi.Chain.ChainHead(ctx)
			if err != nil {
				return err
			}

			nv, err := fapi.Chain.StateNetworkVersion(ctx, types.EmptyTSK)
			if err != nil {
				return err
			}

			extensions := map[miner.SectorLocation]map[abi.ChainEpoch][]uint64{}

			// are given durations within tolerance epochs
			withinTolerance := func(a, b abi.ChainEpoch) bool {
				diff := a - b
				if diff < 0 {
					diff = b - a
				}

				return diff <= abi.ChainEpoch(cctx.Int64("tolerance"))
			}

			sis, err := fapi.Chain.StateMinerActiveSectors(ctx, maddr, types.EmptyTSK)
			if err != nil {
				return fmt.Errorf("getting miner sector infos: %w", err)
			}

			for _, si := range sis {
				if si.SealProof >= abi.RegisteredSealProof_StackedDrg2KiBV1_1 {
					continue
				}

				if si.Expiration < (head.Height() + abi.ChainEpoch(cctx.Int64("expiration-ignore"))) {
					continue
				}

				if cctx.IsSet("expiration-cutoff") {
					if si.Expiration > (head.Height() + abi.ChainEpoch(cctx.Int64("expiration-cutoff"))) {
						continue
					}
				}

				ml := specpolicy.GetSectorMaxLifetime(si.SealProof, nv)
				// if the sector's missing less than "tolerance" of its maximum possible lifetime, don't bother extending it
				if withinTolerance(si.Expiration-si.Activation, ml) {
					continue
				}

				// Set the new expiration to 48 hours less than the theoretical maximum lifetime
				newExp := ml - (miner.WPoStProvingPeriod * 2) + si.Activation
				if withinTolerance(si.Expiration, newExp) || si.Expiration >= newExp {
					continue
				}

				p, err := fapi.Chain.StateSectorPartition(ctx, maddr, si.SectorNumber, types.EmptyTSK)
				if err != nil {
					return fmt.Errorf("getting sector location for sector %d: %w", si.SectorNumber, err)
				}

				if p == nil {
					return fmt.Errorf("sector %d not found in any partition", si.SectorNumber)
				}

				es, found := extensions[*p]
				if !found {
					ne := make(map[abi.ChainEpoch][]uint64)
					ne[newExp] = []uint64{uint64(si.SectorNumber)}
					extensions[*p] = ne
				} else {
					added := false
					for exp := range es {
						if withinTolerance(exp, newExp) && newExp >= exp && exp > si.Expiration {
							es[exp] = append(es[exp], uint64(si.SectorNumber))
							added = true
							break
						}
					}

					if !added {
						es[newExp] = []uint64{uint64(si.SectorNumber)}
					}
				}
			}

			p := core.ExtendSectorExpirationParams{}
			scount := 0

			for l, exts := range extensions {
				for newExp, numbers := range exts {
					scount += len(numbers)
					addressedMax, err := specpolicy.GetAddressedSectorsMax(nv)
					if err != nil {
						return fmt.Errorf("failed to get addressed sectors max")
					}
					declMax, err := specpolicy.GetDeclarationsMax(nv)
					if err != nil {
						return fmt.Errorf("failed to get declarations max")
					}
					if scount > addressedMax || len(p.Extensions) == declMax {
						params = append(params, p)
						p = core.ExtendSectorExpirationParams{}
						scount = len(numbers)
					}

					p.Extensions = append(p.Extensions, core.ExpirationExtension{
						Deadline:      l.Deadline,
						Partition:     l.Partition,
						Sectors:       bitfield.NewFromSet(numbers),
						NewExpiration: newExp,
					})
				}
			}

			// if we have any sectors, then one last append is needed here
			if scount != 0 {
				params = append(params, p)
			}

		} else {
			if !cctx.Args().Present() || !cctx.IsSet("new-expiration") {
				return fmt.Errorf("must pass at least one sector number and new expiration")
			}
			sectors := map[miner.SectorLocation][]uint64{}

			for i, s := range cctx.Args().Slice() {
				id, err := strconv.ParseUint(s, 10, 64)
				if err != nil {
					return fmt.Errorf("could not parse sector %d: %w", i, err)
				}

				p, err := fapi.Chain.StateSectorPartition(ctx, maddr, abi.SectorNumber(id), types.EmptyTSK)
				if err != nil {
					return fmt.Errorf("getting sector location for sector %d: %w", id, err)
				}

				if p == nil {
					return fmt.Errorf("sector %d not found in any partition", id)
				}

				sectors[*p] = append(sectors[*p], id)
			}

			p := core.ExtendSectorExpirationParams{}
			for l, numbers := range sectors {
				p.Extensions = append(p.Extensions, core.ExpirationExtension{
					Deadline:      l.Deadline,
					Partition:     l.Partition,
					Sectors:       bitfield.NewFromSet(numbers),
					NewExpiration: abi.ChainEpoch(cctx.Int64("new-expiration")),
				})
			}

			params = append(params, p)
		}

		if len(params) == 0 {
			fmt.Println("nothing to extend")
			return nil
		}

		mi, err := fapi.Chain.StateMinerInfo(ctx, maddr, types.EmptyTSK)
		if err != nil {
			return fmt.Errorf("getting miner info: %w", err)
		}

		for i := range params {
			sp, aerr := actors.SerializeParams(&params[i])
			if aerr != nil {
				return fmt.Errorf("serializing params: %w", err)
			}

			mid, err := fapi.Messager.PushMessage(ctx, &types.Message{
				From:   mi.Worker,
				To:     maddr,
				Method: miner.Methods.ExtendSectorExpiration,

				Value:  big.Zero(),
				Params: sp,
			}, nil)
			if err != nil {
				return RPCCallError("PushMessageWithId", err)
			}

			fmt.Println(mid)
		}

		return nil
	},
}

var utilSealerSectorsTerminateCmd = &cli.Command{
	Name:      "terminate",
	Usage:     "Terminate sector on-chain (WARNING: This means losing power and collateral for the sector)",
	ArgsUsage: "<sectorNum>",
	Flags: []cli.Flag{
		&cli.Uint64Flag{
			Name:     "actor",
			Required: true,
			Usage:    "actor id, eg. 1000",
		},
		&cli.BoolFlag{
			Name:  "really-do-it",
			Usage: "pass this flag if you know what you are doing",
		},
	},
	Subcommands: []*cli.Command{
		utilSealerSectorsTerminateQueryCmd,
	},
	Action: func(cctx *cli.Context) error {
		if !cctx.Bool("really-do-it") {
			return fmt.Errorf("pass --really-do-it to confirm this action")
		}

		cli, gctx, stop, err := extractAPI(cctx)
		if err != nil {
			return err
		}

		defer stop()

		id, err := strconv.ParseUint(cctx.Args().Get(0), 10, 64)
		if err != nil {
			return fmt.Errorf("could not parse sector number: %w", err)
		}

		actor := cctx.Uint64("actor")
		resp, err := cli.Sealer.TerminateSector(gctx, abi.SectorID{Miner: abi.ActorID(actor), Number: abi.SectorNumber(id)})
		if err != nil {
			return err
		}

		if resp.Res != core.SubmitAccepted {
			fmt.Printf("terminate failed: %s\n", *resp.Desc)
		}
		fmt.Println("terminate accepted")

		return nil
	},
}

var utilSealerSectorsTerminateQueryCmd = &cli.Command{
	Name:      "query",
	Usage:     "Query the terminate info of the specified sector",
	ArgsUsage: "<sectorNum>",
	Action: func(cctx *cli.Context) error {
		cli, gctx, stop, err := extractAPI(cctx)
		if err != nil {
			return err
		}

		defer stop()

		id, err := strconv.ParseUint(cctx.Args().Get(0), 10, 64)
		if err != nil {
			return fmt.Errorf("could not parse sector number: %w", err)
		}

		actor := cctx.Uint64("actor")
		resp, err := cli.Sealer.PollTerminateSectorState(gctx, abi.SectorID{Miner: abi.ActorID(actor), Number: abi.SectorNumber(id)})
		if err != nil {
			return err
		}

		if resp.TerminateCid != nil {
			fmt.Printf("msg: %s, height: %v, added height: %v\n", resp.TerminateCid.String(), resp.TerminatedAt, resp.AddedHeight)
		} else {
			fmt.Printf("msg: null, added height: %v\n", resp.AddedHeight)
		}

		return nil
	},
}

var utilSealerSectorsRemoveCmd = &cli.Command{
	Name:      "remove",
	Usage:     "Forcefully remove persist stores of sector(WARNING: This means losing power and collateral for the removed sector (use 'terminate' for lower penalty))",
	ArgsUsage: "<sectorNum>",
	Flags: []cli.Flag{
		&cli.Uint64Flag{
			Name:     "actor",
			Required: true,
			Usage:    "actor id, eg. 1000",
		},
		&cli.BoolFlag{
			Name:  "really-do-it",
			Usage: "pass this flag if you know what you are doing",
		},
	},
	Action: func(cctx *cli.Context) error {
		if !cctx.Bool("really-do-it") {
			return fmt.Errorf("pass --really-do-it to confirm this action")
		}

		cli, gctx, stop, err := extractAPI(cctx)
		if err != nil {
			return err
		}

		defer stop()

		id, err := strconv.ParseUint(cctx.Args().Get(0), 10, 64)
		if err != nil {
			return fmt.Errorf("could not parse sector number: %w", err)
		}

		actor := cctx.Uint64("actor")
		err = cli.Sealer.RemoveSector(gctx, abi.SectorID{Miner: abi.ActorID(actor), Number: abi.SectorNumber(id)})
		if err != nil {
			return err
		}

		fmt.Println("remove succeed")
		return nil
	},
}
