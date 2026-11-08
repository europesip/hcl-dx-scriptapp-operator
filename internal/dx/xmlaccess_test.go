package dx

import (
	"strings"
	"testing"
)

func TestPageUpdateXML(t *testing.T) {
	ord := int32(900)
	p := Page{UniqueName: "dxop.ns.page", Parent: "ibm.portal.Home", FriendlyName: "my-page", Titles: map[string]string{"es": "Página", "en": "Page & co"},
		ContentIDs: []string{"id-1", "id-2"}, Ordinal: &ord, LayoutTemplate: "dav:x", Skin: "ibm.portal.85NoSkin", Anonymous: true, Authenticated: true, Hash: "sha256:abc"}
	x := PageUpdateXML(p, true)
	for _, want := range []string{`uniquename="dxop.ns.page"`, `ordinal="900"`, `<title>Page &amp; co</title>`, `locale="es"`,
		`name="dxop.hash"`, "sha256:abc", "WCM_CONTENT_CONTEXT_IDR", "id-1", "id-2", `subjectid="anonymous portal user" subjecttype="user" update="set"`} {
		if !strings.Contains(x, want) {
			t.Errorf("missing %q", want)
		}
	}
	if strings.Index(x, "id-1") > strings.Index(x, "id-2") {
		t.Error("applications must keep their order")
	}
	if y := PageUpdateXML(p, false); strings.Contains(y, "portletinstance") {
		t.Error("without layout the components must not be sent (they would be duplicated)")
	}
	p.Anonymous = false
	if y := PageUpdateXML(p, false); !strings.Contains(y, `subjectid="anonymous portal user" subjecttype="user" update="remove"`) {
		t.Error("anonymous access must be removed")
	}
}

func TestParsePageExport(t *testing.T) {
	notFound := `<request><status element="" result="failed"><message id="EJPXA0240E">EJPXA0240E: The resource [content-node uniquename=x] was not found</message></status></request>`
	if e, err := ParsePageExport(notFound, "x"); err != nil || e.Exists {
		t.Errorf("not found: %+v %v", e, err)
	}
	other := `<request><status element="" result="failed"><message id="EJPXA0001E">something else</message></status></request>`
	if _, err := ParsePageExport(other, "x"); err == nil {
		t.Error("another failure must be an error, not a missing page")
	}
	ok := `<request><portal><content-node uniquename="x"><parameter name="dxop.hash" type="string" update="set"><![CDATA[sha256:1]]></parameter></content-node></portal><status element="all" result="ok"/></request>`
	if e, err := ParsePageExport(ok, "x"); err != nil || !e.Exists || e.Hash != "sha256:1" {
		t.Errorf("exists: %+v %v", e, err)
	}
}

func TestRedact(t *testing.T) {
	if got := redact("user wpsadmin pass s3cr3t", "s3cr3t"); strings.Contains(got, "s3cr3t") {
		t.Error("the password must not appear in logs")
	}
}
