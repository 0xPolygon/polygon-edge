package tui

import (
	"context"
	"fmt"
	"time"

	"github.com/0xPolygon/polygon-edge/consensus/polybft/proto"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
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

	primitive := getPrimitive2(clt, app)

	header := tview.NewFlex()
	footer := tview.NewFlex()
	body := tview.NewFlex()

	body.AddItem(primitive, 0, 1, false)

	mainPage := tview.NewFlex().SetDirection(tview.FlexRow)
	mainPage.
		AddItem(header, 0, 4, false).
		AddItem(body, 0, 12, false).
		AddItem(footer, 0, 0, false)

	pages := tview.NewPages()
	pages.AddPage("NameMainPage", mainPage, true, true)

	if err := app.SetRoot(body, true).EnableMouse(true).Run(); err != nil {
		panic(err)
	}
}

func getPrimitive2(clt proto.PolybftClient, app *tview.Application) tview.Primitive {
	resp, _ := clt.Bridge(context.Background(), &proto.BridgeRequest{})

	syncTable := newSyncTable()
	syncTable.draw(resp)

	go func() {
		// redraw
		for {
			time.Sleep(100 * time.Millisecond)
			resp, _ := clt.Bridge(context.Background(), &proto.BridgeRequest{})
			syncTable.draw(resp)
			app.Draw()
		}
	}()

	return syncTable.table
}

func getPrimitive(clt proto.PolybftClient, app *tview.Application) tview.Primitive {
	resp, _ := clt.ConsensusSnapshot(context.Background(), &proto.ConsensusSnapshotRequest{})

	validatorTable := newValidatorsTable()
	validatorTable.draw(resp.Validators)

	go func() {
		// redraw
		for {
			time.Sleep(100 * time.Millisecond)
			resp, _ := clt.ConsensusSnapshot(context.Background(), &proto.ConsensusSnapshotRequest{})
			validatorTable.draw(resp.Validators)
			app.Draw()
		}
	}()

	return validatorTable.table
}

var (
	StandardColorHex          = "#7B3FE4"
	StandardColorTag          = fmt.Sprintf("[%s]", StandardColorHex)
	TcellColorHighlighPrimary = tcell.GetColor("#D0BAF5")
	TcellColorStandard        = tcell.GetColor(StandardColorHex)
)

type validatorsTable struct {
	table *tview.Table
}

func newValidatorsTable() *validatorsTable {
	t := tview.NewTable()
	t.SetBorder(true)
	t.SetTitleColor(TcellColorHighlighPrimary)
	t.SetSelectable(true, false)
	t.SetFixed(1, 1)
	t.SetBorderPadding(0, 0, 1, 1)
	t.SetBorderColor(TcellColorStandard)

	return &validatorsTable{table: t}
}

func (s *validatorsTable) draw(stateSyncs []*proto.Validator) {
	s.table.Clear()

	c := tcell.GetColor(StandardColorHex)
	for i, h := range []string{"Index", "Address", "Balance"} {
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

	for index, sync := range stateSyncs {
		renderRow([]string{
			fmt.Sprintf("%d", index),
			sync.Address,
			fmt.Sprintf("%d", sync.VotingPower),
		})
	}
}

type syncTable struct {
	table *tview.Table
}

func newSyncTable() *syncTable {
	t := tview.NewTable()
	t.SetBorder(true)
	t.SetTitleColor(TcellColorHighlighPrimary)
	t.SetSelectable(true, false)
	t.SetFixed(1, 1)
	t.SetBorderPadding(0, 0, 1, 1)
	t.SetBorderColor(TcellColorStandard)

	return &syncTable{table: t}
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

func (s *syncTable) draw(resp *proto.BridgeResponse) {
	stateSyncs := []*StateSync{}

	resp.StateSyncs = resp.StateSyncs[:10]

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
