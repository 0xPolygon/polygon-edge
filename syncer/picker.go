package syncer

const (
	maxPieceSize = 190
)

type Piece struct {
	start int
	end   int
}

type Picker struct {
	pieces []*Piece
}
