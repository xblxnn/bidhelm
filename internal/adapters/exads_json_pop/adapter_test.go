package exads_json_pop

import (
	"bidhelm/internal/model"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestProvidedContract(t *testing.T) {
	a := Adapter{}
	r, e := a.Parse(httptest.NewRequest("POST", "/", strings.NewReader(`{"id":"d4b5c697-41f3-4c1c-a3d5-5fd01b5ef2aa","ip":"131.34.123.159","language":"en","type":"popunder","remote_addr":"131.34.123.159","x_forwarded_for":"120.52.73.97","ua":"Mozilla/5.0","sub":445566}`)))
	if e != nil {
		t.Fatal(e)
	}
	if r.IP != "131.34.123.159" {
		t.Fatal("used proxy IP")
	}
	w := httptest.NewRecorder()
	a.Respond(w, r, &model.Campaign{URL: "https://example.com", CPM: 130000})
	var out map[string]map[string]any
	if e = json.Unmarshal(w.Body.Bytes(), &out); e != nil {
		t.Fatal(e)
	}
	if out["bid"]["value"] != 0.13 || out["bid"]["btype"] != float64(1) || out["bid"]["id"] != r.Context {
		t.Fatal(out)
	}
	w = httptest.NewRecorder()
	a.Respond(w, r, nil)
	if w.Code != 204 || w.Body.Len() != 0 {
		t.Fatal("no-bid")
	}
}

func TestPlacementParsing(t *testing.T) {
	for _, tc := range []struct{ raw, want string }{
		{`445566`, "445566"}, {`"445566"`, "445566"}, {`"00445566"`, "00445566"}, {`90071992547409931234`, "90071992547409931234"}, {`0`, "0"}, {`" A&б "`, "A&б"},
		{`null`, ""}, {`" "`, ""}, {`true`, ""}, {`[]`, ""}, {`{}`, ""}, {`1.2`, ""}, {`1e3`, ""},
	} {
		r, err := (Adapter{}).Parse(httptest.NewRequest("POST", "/", strings.NewReader(`{"id":"r","type":"popunder","sub":`+tc.raw+`}`)))
		if tc.want == "" {
			if err == nil {
				t.Fatal("accepted", tc.raw)
			}
		} else if err != nil || r.PlacementID != tc.want {
			t.Fatal(tc, r, err)
		}
	}
	if _, err := (Adapter{}).Parse(httptest.NewRequest("POST", "/", strings.NewReader(`{"id":"r","type":"popunder"}`))); err == nil {
		t.Fatal("missing sub accepted")
	}
}
