package config

import (
	"github.com/consensys/gnark-crypto/field"
)

// Curve describes parameters of the curve useful for the template
type Curve struct {
	Name         string
	CurvePackage string
	Package      string // current package being generated
	EnumID       string
	FpModulus    string
	FrModulus    string

	Fp           *field.Field
	Fr           *field.Field
	FpUnusedBits int
	G1           Point
	G2           Point
}

func (conf *Curve) init() {
	conf.Fp, _ = field.NewField("fp", "Element", conf.FpModulus)
	conf.Fr, _ = field.NewField("fr", "Element", conf.FrModulus)
	conf.FpUnusedBits = 64 - (conf.Fp.NbBits % 64)
}

func (c Curve) Equal(other Curve) bool {
	return c.Name == other.Name
}

type Point struct {
	CoordType        string
	PointName        string
	GLV              bool  // scalar mulitplication using GLV
	CofactorCleaning bool  // flag telling if the Cofactor cleaning is available
	CRange           []int // multiexp bucket method: generate inner methods (with const arrays) for each c
	Projective       bool  // generate projective coordinates
}

var Curves []Curve

func defaultCRange() []int {
	// default range for C values in the multiExp
	return []int{4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 20, 21, 22}
}
