package config

var BLS12_381 = Curve{
	Name:         "bls12-381",
	CurvePackage: "bls12381",
	EnumID:       "BLS12_381",
	FrModulus:    "52435875175126190479447740508185965837690552500527637822603658699938581184513",
	FpModulus:    "4002409555221667393417789825735904156556882819939007885332058136124031650490837864442687629129015664037894272559787",
	G1: Point{
		CoordType:        "fp.Element",
		PointName:        "g1",
		GLV:              true,
		CofactorCleaning: true,
		CRange:           defaultCRange(),
	},
	G2: Point{
		CoordType:        "fptower.E2",
		PointName:        "g2",
		GLV:              true,
		CofactorCleaning: true,
		CRange:           defaultCRange(),
		Projective:       true,
	},
}

func init() {
	addCurve(&BLS12_381)

}
