package webdav

import (
	"strings"
	"testing"
)

func TestParsePropfindBody(t *testing.T) {
	req, err := parsePropfindBody([]byte(`<?xml version="1.0"?>
		<D:propfind xmlns:D="DAV:"><D:prop>
			<D:getlastmodified/><x:foo xmlns:x="urn:test"/>
		</D:prop></D:propfind>`))
	if err != nil {
		t.Fatalf("parsePropfindBody: %v", err)
	}
	if req.AllProp || req.PropName {
		t.Fatalf("unexpected flags: %+v", req)
	}
	if len(req.Names) != 2 {
		t.Fatalf("got %d names, want 2: %+v", len(req.Names), req.Names)
	}
	if req.Names[0] != (propName{Space: davNS, Local: "getlastmodified"}) {
		t.Errorf("unexpected first name: %+v", req.Names[0])
	}
	if req.Names[1] != (propName{Space: "urn:test", Local: "foo"}) {
		t.Errorf("unexpected second name: %+v", req.Names[1])
	}
}

func TestParsePropfindAllPropWithExtension(t *testing.T) {
	req, err := parsePropfindBody([]byte(`<propfind xmlns="DAV:"><foobar/><allprop/></propfind>`))
	if err != nil {
		t.Fatalf("parsePropfindBody: %v", err)
	}
	if !req.AllProp {
		t.Fatal("AllProp not set")
	}
}

func TestParsePropfindRejectsMalformedXML(t *testing.T) {
	if _, err := parsePropfindBody([]byte(`<foo>`)); err == nil {
		t.Fatal("expected malformed XML to be rejected")
	}
	if _, err := parsePropfindBody([]byte(`<D:propfind xmlns:D="DAV:"><D:prop><bar:foo xmlns:bar=""/></D:prop></D:propfind>`)); err == nil {
		t.Fatal("expected empty namespace declaration to be rejected")
	}
}

func TestParseProppatchPreservesOrder(t *testing.T) {
	body := `<?xml version="1.0"?><propertyupdate xmlns="DAV:">
		<remove><prop><removeset xmlns="urn:test"/></prop></remove>
		<set><prop><removeset xmlns="urn:test">x</removeset></prop></set>
		<set><prop><removeset xmlns="urn:test">y</removeset></prop></set>
	</propertyupdate>`
	ops, err := parseProppatchBody([]byte(body))
	if err != nil {
		t.Fatalf("parseProppatchBody: %v", err)
	}
	if len(ops) != 3 {
		t.Fatalf("got %d ops, want 3", len(ops))
	}
	if !ops[0].Remove || ops[0].Value.Text != "" {
		t.Errorf("unexpected first op: %+v", ops[0])
	}
	if ops[1].Remove || ops[1].Value.Text != "x" {
		t.Errorf("unexpected second op: %+v", ops[1])
	}
	if ops[2].Remove || ops[2].Value.Text != "y" {
		t.Errorf("unexpected third op: %+v", ops[2])
	}
}

func TestParseProppatchStructuredValue(t *testing.T) {
	body := `<propertyupdate xmlns="DAV:"><set><prop>
		<valnspace xmlns="urn:test"><foo xmlns="http://bar"/></valnspace>
	</prop></set></propertyupdate>`
	ops, err := parseProppatchBody([]byte(body))
	if err != nil {
		t.Fatalf("parseProppatchBody: %v", err)
	}
	if len(ops) != 1 {
		t.Fatalf("got %d ops, want 1", len(ops))
	}
	if strings.Count(ops[0].Value.XML, "xmlns=") != 1 {
		t.Errorf("structured value has duplicate xmlns: %s", ops[0].Value.XML)
	}
}

func TestDeadPropertiesMoveAndDelete(t *testing.T) {
	h := New(nil, "", "")
	name := propName{Space: "urn:test", Local: "foo"}
	h.setDead("a/b", name, deadProp{Text: "v"})
	h.setDead("a/b/child", name, deadProp{Text: "c"})

	h.moveDeadProps("a/b", "x/y")
	if _, ok := h.deadFor("a/b")[name]; ok {
		t.Error("old path still has properties")
	}
	if got := h.deadFor("x/y")[name].Text; got != "v" {
		t.Errorf("moved value = %q, want v", got)
	}
	if got := h.deadFor("x/y/child")[name].Text; got != "c" {
		t.Errorf("moved child value = %q, want c", got)
	}

	h.deleteDeadProps("x/y")
	if got := h.deadFor("x/y/child"); len(got) != 0 {
		t.Errorf("child properties not deleted: %+v", got)
	}
}
