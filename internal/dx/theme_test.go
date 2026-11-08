package dx

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseDAVList(t *testing.T) {
	xml := []byte(`<D:multistatus xmlns:D="DAV:">
<D:response><D:href>/wps/mycontenthandler/dav/fs-type1/themes/Portal8.5/</D:href></D:response>
<D:response><D:href>/wps/mycontenthandler/dav/fs-type1/themes/Portal8.5/theme.html</D:href></D:response>
<D:response><D:href>/wps/mycontenthandler/dav/fs-type1/themes/Portal8.5/css/</D:href></D:response>
<D:response><D:href>/wps/mycontenthandler/dav/fs-type1/themes/Portal8.5/nls/theme_es.html</D:href></D:response>
</D:multistatus>`)
	got := parseDAVList(xml, "Portal8.5/")
	want := []string{"Portal8.5/theme.html", "Portal8.5/css/", "Portal8.5/nls/theme_es.html"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("got %v, want %v", got, want)
	}
}

const baseExport = `<?xml version="1.0" encoding="UTF-8"?>
<request build="wp9500" version="9.5.0.0" type="export" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xsi:noNamespaceSchemaLocation="PortalConfig_8.5.0.xsd">
  <portal action="locate">
    <theme action="update" active="true" default="true" domain="rel" objectid="Z6_ABC" uniquename="ibm.portal.85Theme" context-root="/wps/defaultTheme85">
      <localedata locale="en"><title>Portal 8.5</title></localedata>
      <localedata locale="de"><title>Portal 8.5</title></localedata>
      <parameter name="com.ibm.portal.theme.template.ref" type="string" update="set"><![CDATA[dav:fs-type1/themes/Portal8.5/]]></parameter>
      <parameter name="com.ibm.portal.isTemplate" type="string" update="set"><![CDATA[true]]></parameter>
      <parameter name="com.ibm.portal.theme.disallow" type="string" update="set"><![CDATA[x]]></parameter>
    </theme>
  </portal>
  <status element="all" result="ok"/>
</request>`

func TestThemeRegisterXML(t *testing.T) {
	x, err := ThemeRegisterXML(baseExport, ThemeRegistration{UniqueName: "dxop.ns.blue", Folder: "dxop-blue", BaseUniqueName: "ibm.portal.85Theme",
		BaseFolder: "Portal8.5", Titles: map[string]string{"en": "Blue", "es": "Azul"}, Hash: "sha256:1"})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`uniquename="dxop.ns.blue"`, `objectid="dxopTheme"`, `default="false"`, `dav:fs-type1/themes/dxop-blue/`,
		`<![CDATA[false]]>`, `<title>Blue</title>`, `<title>Azul</title>`, `type="update" create-oids="true"`, `<![CDATA[sha256:1]]>`} {
		if !strings.Contains(x, want) {
			t.Errorf("missing %s in\n%s", want, x)
		}
	}
	for _, bad := range []string{`Portal8.5/`, `theme.disallow`, `locale="de"`, `<status`, `build=`, `ibm.portal.85Theme`} {
		if strings.Contains(x, bad) {
			t.Errorf("%s should be gone from\n%s", bad, x)
		}
	}
}

func TestParseThemeExport(t *testing.T) {
	if e, err := ParseThemeExport(`<request><status element="" result="failed"><message id="EJPXA0240E">not found</message></status></request>`, "x"); err != nil || e.Exists {
		t.Fatalf("not found: %v %v", e, err)
	}
	ok := `<request><portal><theme uniquename="x"><parameter name="dxop.hash" type="string" update="set"><![CDATA[sha256:9]]></parameter></theme></portal><status element="all" result="ok"/></request>`
	if e, err := ParseThemeExport(ok, "x"); err != nil || !e.Exists || e.Hash != "sha256:9" {
		t.Fatalf("found: %v %v", e, err)
	}
	if _, err := ParseThemeExport(`<request><status element="" result="failed"><message>boom</message></status></request>`, "x"); err == nil {
		t.Fatal("a failure that is not 'not found' must be an error")
	}
}

func TestAddToHead(t *testing.T) {
	dir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(dir, "nls"), 0o755)
	_ = os.WriteFile(filepath.Join(dir, "nls", "theme_es.html"), []byte("<html><head><title>x</title></head></html>"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "theme.html"), []byte("<html><head></head></html>"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "other.html"), []byte("<html><head></head></html>"), 0o644)
	n, err := AddToHead(dir, "<style>.x{}</style>")
	if err != nil || n != 2 {
		t.Fatalf("changed %d, err %v", n, err)
	}
	if n, _ := AddToHead(dir, "<style>.x{}</style>"); n != 0 {
		t.Fatal("must not add it twice")
	}
	other, _ := os.ReadFile(filepath.Join(dir, "other.html"))
	if strings.Contains(string(other), "style") {
		t.Fatal("only the theme templates")
	}
}

func TestThemeRegisterXMLProfileAndLayout(t *testing.T) {
	export := strings.Replace(baseExport, `<parameter name="com.ibm.portal.isTemplate"`, `<parameter name="resourceaggregation.profile" type="string" update="set"><![CDATA[profiles/profile_deferred.json]]></parameter>
      <parameter name="com.ibm.portal.isTemplate"`, 1)
	x, err := ThemeRegisterXML(export, ThemeRegistration{UniqueName: "europesip.sample", Folder: "europesip", BaseUniqueName: "ibm.portal.85Theme",
		BaseFolder: "Portal8.5", Titles: map[string]string{"en": "EUROPESIP"}, Hash: "h", Profile: "profiles/profile_europesip.json", Layout: "layout-templates/1Column"})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`<![CDATA[profiles/profile_europesip.json]]>`, `name="com.ibm.portal.layout.template.href" type="string" update="set"><![CDATA[dav:fs-type1/themes/europesip/layout-templates/1Column/]]>`} {
		if !strings.Contains(x, want) {
			t.Errorf("missing %s in\n%s", want, x)
		}
	}
	if strings.Contains(x, "profile_deferred") {
		t.Error("the base profile must be replaced")
	}
}
