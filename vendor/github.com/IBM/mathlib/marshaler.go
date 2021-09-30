/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package math

import "encoding/json"

type curveElement struct {
	CurveID      CurveID `json:"curve" validate:"required"`
	ElementBytes []byte  `json:"element" validate:"required"`
}

func (z *Zr) UnmarshalJSON(raw []byte) error {
	ce := &curveElement{}
	err := json.Unmarshal(raw, ce)
	if err != nil {
		return err
	}

	z.curveID = ce.CurveID
	z.zr = Curves[z.curveID].NewZrFromBytes(ce.ElementBytes).zr

	return nil
}

func (z *Zr) MarshalJSON() ([]byte, error) {
	return json.Marshal(&curveElement{
		CurveID:      z.curveID,
		ElementBytes: z.Bytes(),
	})
}

func (g *G1) UnmarshalJSON(raw []byte) error {
	ce := &curveElement{}
	err := json.Unmarshal(raw, ce)
	if err != nil {
		return err
	}

	g.curveID = ce.CurveID
	g1, err := Curves[g.curveID].NewG1FromBytes(ce.ElementBytes)
	if err != nil {
		return err
	}

	g.g1 = g1.g1
	return nil
}

func (g *G1) MarshalJSON() ([]byte, error) {
	return json.Marshal(&curveElement{
		CurveID:      g.curveID,
		ElementBytes: g.Bytes(),
	})
}

func (g *G2) UnmarshalJSON(raw []byte) error {
	ce := &curveElement{}
	err := json.Unmarshal(raw, ce)
	if err != nil {
		return err
	}

	g.curveID = ce.CurveID
	g2, err := Curves[g.curveID].NewG2FromBytes(ce.ElementBytes)
	if err != nil {
		return err
	}

	g.g2 = g2.g2
	return nil
}

func (g *G2) MarshalJSON() ([]byte, error) {
	return json.Marshal(&curveElement{
		CurveID:      g.curveID,
		ElementBytes: g.Bytes(),
	})
}

func (g *Gt) UnmarshalJSON(raw []byte) error {
	ce := &curveElement{}
	err := json.Unmarshal(raw, ce)
	if err != nil {
		return err
	}

	g.curveID = ce.CurveID
	gt, err := Curves[g.curveID].NewGtFromBytes(ce.ElementBytes)
	if err != nil {
		return err
	}

	g.gt = gt.gt
	return nil
}

func (g *Gt) MarshalJSON() ([]byte, error) {
	return json.Marshal(&curveElement{
		CurveID:      g.curveID,
		ElementBytes: g.Bytes(),
	})
}
