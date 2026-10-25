/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// marshalIndentJSON uses json.Deterministic(true), jsontext.EscapeForHTML(true),
// json.FormatNilSliceAsNull(true), json.FormatNilMapAsNull(true) with tab indent.
// toJSON uses plain json.Marshal (no options): v2 does not escape HTML by
// default and does not apply FormatNil*AsNull.
func TestMarshalIndentJSONV2Behavior(t *testing.T) {
	t.Run("deterministic sorted keys with tab indent and HTML escaped", func(t *testing.T) {
		v := map[string]any{
			"b": "x<y>&z",
			"a": map[string]any{"d": 1, "c": 2},
		}
		b, err := marshalIndentJSON(v)
		require.NoError(t, err)
		expected := `{
	"a": {
		"c": 2,
		"d": 1
	},
	"b": "x\u003cy\u003e\u0026z"
}`
		require.Equal(t, expected, string(b))
	})

	t.Run("nil slice and nil map serialize as null", func(t *testing.T) {
		type s struct {
			S []string `json:"s"`
			M map[string]int
		}
		b, err := marshalIndentJSON(s{})
		require.NoError(t, err)
		require.Contains(t, string(b), `"s": null`)
		require.Contains(t, string(b), `"M": null`)
	})

	t.Run("zero values emitted with omitempty absent", func(t *testing.T) {
		type s struct {
			Sequence     int64  `json:"sequence"`
			InitRequired bool   `json:"init_required"`
			Name         string `json:"name"`
		}
		b, err := marshalIndentJSON(s{})
		require.NoError(t, err)
		require.Contains(t, string(b), `"sequence": 0`)
		require.Contains(t, string(b), `"init_required": false`)
		require.Contains(t, string(b), `"name": ""`)
	})
}

func TestToJSONV2Behavior(t *testing.T) {
	// toJSON escapes HTML characters so that package IDs match the ones
	// produced before the encoding/json v2 bump.
	t.Run("HTML characters are escaped", func(t *testing.T) {
		b, err := toJSON("pkg<name>", "golang", "label&x")
		require.NoError(t, err)
		require.Contains(t, string(b), `pkg\u003cname\u003e`)
		require.Contains(t, string(b), `label\u0026x`)
	})
}
