package tui

import (
	"context"
	"encoding/hex"
	"fmt"
	"math/big"
	"time"

	"github.com/0xPolygon/polygon-edge/consensus/polybft/proto"
	"github.com/0xPolygon/polygon-edge/contracts"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/abi"
	"github.com/umbracle/ethgo/jsonrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func NewView() {
	client, err := grpc.Dial("localhost:5001", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		panic(err)
	}

	clt := proto.NewPolybftClient(client)

	app := tview.NewApplication()
	//main := getPrimitive3(clt, app)

	updateCh := make(chan string)

	// create the table
	table := newTable()
	table.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyCtrlH {
			updateCh <- "checkpoint"
		} else if event.Key() == tcell.KeyCtrlS {
			updateCh <- "stateSync"
		} else if event.Key() == tcell.KeyCtrlR {
			updateCh <- "validator"
		}

		return event
	})

	// create the components
	var (
		checkPointTable = newCheckpointTable(table)
		stateSyncTable  = newSyncTable(clt, table)
		validatorTable  = newValidatorsTable(clt, table)
	)

	whichTable := "validator"

	go func() {
		for {
			table.Clear()

			switch whichTable {
			case "checkpoint":
				checkPointTable.draw()
			case "stateSync":
				stateSyncTable.draw()
			case "validator":
				validatorTable.draw()
			}
			app.Draw()

			select {
			case newState := <-updateCh:
				whichTable = newState
			case <-time.After(1 * time.Second):
			}
		}
	}()

	newPrimitive := func(text string) tview.Primitive {
		return tview.NewTextView().
			SetTextAlign(tview.AlignCenter).
			SetText(text)
	}

	grid := tview.NewGrid().
		SetRows(3, 0, 3).
		SetColumns(30, 0, 30).
		SetBorders(true).
		AddItem(newPrimitive("Ctlr-S : State-Sync, Ctlr-R: Validators, Ctlr-H: Checkpoint"), 0, 0, 1, 3, 0, 0, false)

	// Layout for screens narrower than 100 cells (menu and side bar are hidden).
	grid.AddItem(table, 1, 0, 2, 3, 0, 0, false)

	if err := app.SetRoot(grid, true).EnableMouse(true).Run(); err != nil {
		panic(err)
	}

}

var (
	StandardColorHex          = "#7B3FE4"
	StandardColorTag          = fmt.Sprintf("[%s]", StandardColorHex)
	TcellColorHighlighPrimary = tcell.GetColor("#D0BAF5")
	TcellColorStandard        = tcell.GetColor(StandardColorHex)
)

type validatorsTable struct {
	clt   proto.PolybftClient
	table *tview.Table
}

func newValidatorsTable(clt proto.PolybftClient, table *tview.Table) *validatorsTable {
	return &validatorsTable{clt: clt, table: table}
}

type validator struct {
	stake   *big.Int
	rewards *big.Int
}

func getvalidator(addr ethgo.Address) *validator {
	jsonrpcClient, err := jsonrpc.NewClient("http://localhost:9545")
	if err != nil {
		panic(err)
	}

	checkpointManagerAddr := ethgo.Address(contracts.ValidatorSetContract)

	getValidatorMethod := abi.MustNewMethod("function getValidator(address) returns (tuple(uint256[4],uint256,uint256,uint256,uint256,bool))")

	input, err := getValidatorMethod.Encode([]interface{}{addr})
	if err != nil {
		panic(err)
	}

	res, err := jsonrpcClient.Eth().Call(&ethgo.CallMsg{
		To:   &checkpointManagerAddr,
		Data: input,
	}, ethgo.Latest)
	if err != nil {
		panic(err)
	}

	output, err := hex.DecodeString(res[2:])
	if err != nil {
		panic(err)
	}

	xxx, err := getValidatorMethod.Decode(output)
	if err != nil {
		panic(err)
	}
	xx := xxx["0"].(map[string]interface{})

	val := &validator{
		stake:   xx["1"].(*big.Int),
		rewards: xx["4"].(*big.Int),
	}
	return val
}

func (s *validatorsTable) draw() {

	resp, err := s.clt.ConsensusSnapshot(context.Background(), &proto.ConsensusSnapshotRequest{})
	if err != nil {
		panic(err)
	}
	stateSyncs := resp.Validators

	s.table.Clear()

	c := tcell.GetColor(StandardColorHex)
	for i, h := range []string{"Index", "Address", "Voting Power", "Rewards", "Balance"} {
		s.table.SetCell(0, i, tview.NewTableCell(h).
			SetTextColor(c).
			SetSelectable(false),
		)
	}

	index := 1
	renderRow := func(data []string) {
		for i, r := range data {
			s.table.SetCell(index, i,
				tview.NewTableCell(r).SetExpansion(1),
			)
		}
		index++
	}

	jsonrpcClient, err := jsonrpc.NewClient("http://localhost:9545")
	if err != nil {
		panic(err)
	}

	for index, sync := range stateSyncs {
		validator := getvalidator(ethgo.HexToAddress(sync.Address))

		balance, err := jsonrpcClient.Eth().GetBalance(ethgo.HexToAddress(sync.Address), ethgo.Latest)
		if err != nil {
			panic(err)
		}

		renderRow([]string{
			fmt.Sprintf("%d", index),
			sync.Address,
			fmt.Sprintf("%s", validator.stake.String()),
			fmt.Sprintf("%s", validator.rewards.String()),
			fmt.Sprintf("%s", balance.String()),
		})
	}
}

func newTable() *tview.Table {
	t := tview.NewTable()
	t.SetBorder(true)
	t.SetTitleColor(TcellColorHighlighPrimary)
	t.SetSelectable(true, false)
	t.SetFixed(1, 1)
	t.SetBorderPadding(0, 0, 1, 1)
	t.SetBorderColor(TcellColorStandard)

	return t
}

type syncTable struct {
	clt   proto.PolybftClient
	table *tview.Table
}

func newSyncTable(clt proto.PolybftClient, table *tview.Table) *syncTable {
	return &syncTable{clt: clt, table: table}
}

var memPoolHeaders = []string{
	"Id",
	"Target",
	"Status",
}

type StateSync struct {
	ID     uint64
	Target string
	Status string
}

func (s *syncTable) draw() {
	resp, err := s.clt.Bridge(context.Background(), &proto.BridgeRequest{})
	if err != nil {
		panic(err)
	}

	stateSyncs := []*StateSync{}

	for _, i := range resp.StateSyncs {
		var status string
		if i.Id > resp.LastCommittedIndex {
			status = "pending"
		} else {
			if i.Id < resp.NextExecutionIndex {
				status = "executed"
			} else {
				status = "committed"
			}
		}

		stateSyncs = append(stateSyncs, &StateSync{
			ID:     i.Id,
			Target: i.Target,
			Status: status,
		})
	}

	s.table.Clear()

	c := tcell.GetColor(StandardColorHex)
	for i, h := range memPoolHeaders {
		s.table.SetCell(0, i, tview.NewTableCell(h).
			SetTextColor(c).
			SetSelectable(false),
		)
	}

	statusIndx := 0

	for indx, i := range memPoolHeaders {
		if i == "Status" {
			statusIndx = indx
		}
	}

	index := 1
	renderRow := func(data []string) {
		for i, r := range data {
			cell := tview.NewTableCell(r).SetExpansion(1)

			if i == statusIndx {
				switch r {
				case "pending":
					cell = cell.SetTextColor(tcell.ColorWheat)
				case "executed":
					cell = cell.SetTextColor(tcell.ColorOrange)
				case "committed":
					cell = cell.SetTextColor(tcell.ColorGreen)
				}
			}

			s.table.SetCell(index, i,
				cell,
			)
		}
		index++
	}

	for _, sync := range stateSyncs {
		renderRow([]string{
			fmt.Sprintf("%d", sync.ID),
			sync.Target,
			sync.Status,
		})
	}
}

/// -------

type checkpointTable struct {
	table *tview.Table
}

func newCheckpointTable(table *tview.Table) *checkpointTable {
	return &checkpointTable{table: table}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func (s *checkpointTable) draw() {
	clt, err := jsonrpc.NewClient("http://localhost:8545")
	if err != nil {
		panic(err)
	}

	checkpoints := []*checkpoint{}
	for i := uint64(0); i <= getCurrentEpoch(clt); i++ {
		checkpoints = append(checkpoints, getCheckpointByNumber(clt, i))
	}

	s.table.Clear()

	c := tcell.GetColor(StandardColorHex)
	for i, h := range []string{"Epoch", "Block number", "Exit root"} {
		s.table.SetCell(0, i, tview.NewTableCell(h).
			SetTextColor(c).
			SetSelectable(false),
		)
	}

	index := 1
	renderRow := func(data []string) {
		for i, r := range data {
			s.table.SetCell(index, i,
				tview.NewTableCell(r).SetExpansion(1),
			)
		}
		index++
	}

	for _, sync := range checkpoints {
		renderRow([]string{
			fmt.Sprintf("%d", sync.Epoch),
			fmt.Sprintf("%d", sync.BlockNumber),
			fmt.Sprintf("0x%s", hex.EncodeToString(sync.EventRoot[:])),
		})
	}
}

/*

 */
