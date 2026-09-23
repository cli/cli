package recording

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDecode(t *testing.T) {
	for _, tc := range []struct {
		name, source, timing string
		bad                  bool
	}{
		{name: "omitted timing", source: `{"formats":["mp4"]}`, timing: "condensed"},
		{name: "explicit realtime", source: `{"formats":["gif"],"timing":"realtime"}`, timing: "realtime"},
		{name: "blank timing", source: `{"timing":""}`, bad: true},
		{name: "null timing", source: `{"timing":null}`, bad: true},
		{name: "non-string timing", source: `{"timing":2}`, bad: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var value OutputOptions
			err := json.Unmarshal([]byte(tc.source), &value)
			if tc.bad {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.timing, value.Timing)
		})
	}
	t.Run("receipt shape", func(t *testing.T) {
		source := `{"schemaVersion":1,"status":"ready","manifestSha256":"manifest","tools":{
			"node":{"path":"/node","version":"v1","sha256":"node"},
			"tuistory":{"moduleRoot":"/modules","version":"0.11.0","packageSha256":"package"},
			"ffmpeg":{"path":"/ffmpeg","version":"v1","sha256":"ffmpeg"},
			"ffprobe":{"path":"/ffprobe","version":"v1","sha256":"ffprobe"}},
			"checks":{"formats":{"gif":true},"nativePty":true}}`
		var receipt Receipt
		require.NoError(t, json.Unmarshal([]byte(source), &receipt))
		data, err := json.Marshal(receipt)
		require.NoError(t, err)
		require.JSONEq(t, source, string(data))
		require.Equal(t, receipt.Tools.FFmpeg, receipt.Tools.Executable("ffmpeg"))
		require.Nil(t, receipt.Tools.Executable("unknown"))
	})
	t.Run("unselected installation remains explicit", func(t *testing.T) {
		data, err := json.Marshal(Tools{Tuistory: &Tuistory{ModuleRoot: ""}})
		require.NoError(t, err)
		require.JSONEq(t, `{"tuistory":{"moduleRoot":""}}`, string(data))
	})
}

func TestVisibleText(t *testing.T) {
	for _, tc := range []struct {
		name, source, want string
		bad                bool
	}{
		{name: "viewport excludes scrollback", source: `{"cols":40,"rows":1,"lines":[
			{"spans":[{"text":"history"}]},{"spans":[{"text":"visible "},{"text":"text   "}]}]}`, want: "visible text"},
		{name: "empty text is valid", source: `{"cols":40,"rows":1,"lines":[{"spans":[{"text":""}]}]}`},
		{name: "missing span text", source: `{"cols":40,"rows":1,"lines":[{"spans":[{}]}]}`, bad: true},
		{name: "null span text", source: `{"cols":40,"rows":1,"lines":[{"spans":[{"text":null}]}]}`, bad: true},
		{name: "missing rows", source: `{"cols":40,"lines":[]}`, bad: true},
		{name: "null lines", source: `{"cols":40,"rows":1,"lines":null}`, bad: true},
		{name: "null spans", source: `{"cols":40,"rows":1,"lines":[{"spans":null}]}`, bad: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var value TerminalData
			err := json.Unmarshal([]byte(tc.source), &value)
			var actual string
			if err == nil {
				actual, err = value.VisibleText()
			}
			if tc.bad {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, actual)
		})
	}
}
