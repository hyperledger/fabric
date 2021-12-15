package config

var BN254 = Curve{
	Name:         "bn254",
	CurvePackage: "bn254",
	EnumID:       "BN254",
	FrModulus:    "21888242871839275222246405745257275088548364400416034343698204186575808495617",
	FpModulus:    "21888242871839275222246405745257275088696311157297823662689037894645226208583",
	G1: Point{
		CoordType:        "fp.Element",
		PointName:        "g1",
		GLV:              true,
		CofactorCleaning: false,
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
	addCurve(&BN254)
}
