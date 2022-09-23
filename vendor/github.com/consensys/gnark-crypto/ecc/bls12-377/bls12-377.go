package bls12377

import (
	"math/big"

	"github.com/consensys/gnark-crypto/ecc"
	"github.com/consensys/gnark-crypto/ecc/bls12-377/fp"
	"github.com/consensys/gnark-crypto/ecc/bls12-377/fr"
	"github.com/consensys/gnark-crypto/ecc/bls12-377/internal/fptower"
)

// E: y**2=x**3+1
// Etwist: y**2 = x**3+u**-1
// Tower: Fp->Fp2, u**2=-5 -> Fp12, v**6=u
// Generator (BLS12 family): x=9586122913090633729
// optimal Ate loop: trace(frob)-1=x
// trace of pi: x+1
// Fp: p=258664426012969094010652733694893533536393512754914660539884262666720468348340822774968888139573360124440321458177
// Fr: r=8444461749428370424248824938781546531375899335154063827935233455917409239041 (x**4-x**2+1)

// ID bls377 ID
const ID = ecc.BLS12_377

// bCurveCoeff b coeff of the curve
var bCurveCoeff fp.Element

// twist
var twist fptower.E2

// bTwistCurveCoeff b coeff of the twist (defined over Fp2) curve
var bTwistCurveCoeff fptower.E2

// generators of the r-torsion group, resp. in ker(pi-id), ker(Tr)
var g1Gen G1Jac
var g2Gen G2Jac

var g1GenAff G1Affine
var g2GenAff G2Affine

// point at infinity
var g1Infinity G1Jac
var g2Infinity G2Jac

// optimal Ate loop counter (=trace-1 = x in BLS family)
var loopCounter [64]int8

// Parameters useful for the GLV scalar multiplication. The third roots define the
//  endomorphisms phi1 and phi2 for <G1Affine> and <G2Affine>. lambda is such that <r, phi-lambda> lies above
// <r> in the ring Z[phi]. More concretely it's the associated eigenvalue
// of phi1 (resp phi2) restricted to <G1Affine> (resp <G2Affine>)
// cf https://www.cosic.esat.kuleuven.be/nessie/reports/phase2/GLV.pdf
var thirdRootOneG1 fp.Element
var thirdRootOneG2 fp.Element
var lambdaGLV big.Int

// glvBasis stores R-linearly independant vectors (a,b), (c,d)
// in ker((u,v)->u+vlambda[r]), and their determinant
var glvBasis ecc.Lattice

// psi o pi o psi**-1, where psi:E->E' is the degree 6 iso defined over Fp12
var endo struct {
	u fptower.E2
	v fptower.E2
}

// generator of the curve
var xGen big.Int

// expose the tower -- github.com/consensys/gnark uses it in a gnark circuit

// E2 is a degree two finite field extension of fp.Element
type E2 = fptower.E2

// E6 is a degree three finite field extension of fp2
type E6 = fptower.E6

// E12 is a degree two finite field extension of fp6
type E12 = fptower.E12

func init() {

	bCurveCoeff.SetUint64(1)
	twist.A1.SetUint64(1)
	bTwistCurveCoeff.Inverse(&twist)

	g1Gen.X.SetString("81937999373150964239938255573465948239988671502647976594219695644855304257327692006745978603320413799295628339695")
	g1Gen.Y.SetString("241266749859715473739788878240585681733927191168601896383759122102112907357779751001206799952863815012735208165030")
	g1Gen.Z.SetString("1")

	g2Gen.X.SetString("233578398248691099356572568220835526895379068987715365179118596935057653620464273615301663571204657964920925606294",
		"140913150380207355837477652521042157274541796891053068589147167627541651775299824604154852141315666357241556069118")
	g2Gen.Y.SetString("63160294768292073209381361943935198908131692476676907196754037919244929611450776219210369229519898517858833747423",
		"149157405641012693445398062341192467754805999074082136895788947234480009303640899064710353187729182149407503257491")
	g2Gen.Z.SetString("1",
		"0")

	g1GenAff.FromJacobian(&g1Gen)
	g2GenAff.FromJacobian(&g2Gen)

	g1Infinity.X.SetOne()
	g1Infinity.Y.SetOne()
	g2Infinity.X.SetOne()
	g2Infinity.Y.SetOne()

	thirdRootOneG1.SetString("80949648264912719408558363140637477264845294720710499478137287262712535938301461879813459410945")
	thirdRootOneG2.Square(&thirdRootOneG1)
	lambdaGLV.SetString("91893752504881257701523279626832445440", 10) //(x**2-1)
	_r := fr.Modulus()
	ecc.PrecomputeLattice(_r, &lambdaGLV, &glvBasis)

	endo.u.A0.SetString("80949648264912719408558363140637477264845294720710499478137287262712535938301461879813459410946")
	endo.v.A0.SetString("216465761340224619389371505802605247630151569547285782856803747159100223055385581585702401816380679166954762214499")

	// binary decomposition of 15132376222941642752 little endian
	loopCounter = [64]int8{1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 1, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 1, 0, 0, 0, 0, 1}

	xGen.SetString("9586122913090633729", 10)

}

// Generators return the generators of the r-torsion group, resp. in ker(pi-id), ker(Tr)
func Generators() (g1Jac G1Jac, g2Jac G2Jac, g1Aff G1Affine, g2Aff G2Affine) {
	g1Aff = g1GenAff
	g2Aff = g2GenAff
	g1Jac = g1Gen
	g2Jac = g2Gen
	return
}
