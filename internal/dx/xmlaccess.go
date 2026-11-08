package dx

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// Page is a DX page with Script Applications, as the operator creates it with XMLAccess.
type Page struct {
	UniqueName     string
	Parent         string
	FriendlyName   string
	Titles         map[string]string
	ContentIDs     []string // WCM content IDs of the Script Applications, one viewer each, in order
	Ordinal        *int32
	LayoutTemplate string
	Skin           string
	Theme          string
	Anonymous      bool
	Authenticated  bool
	// Hash is stored as a page parameter (dxop.hash): it tells whether the page in DX is the one the operator
	// applied.
	Hash string
}

// HashParameter is the page parameter where the operator keeps the hash of what it applied.
const HashParameter = "dxop.hash"

func esc(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;")
	return r.Replace(s)
}

func cdata(s string) string {
	return "<![CDATA[" + strings.ReplaceAll(s, "]]>", "]]]]><![CDATA[>") + "]]>"
}

func param(k, v string) string {
	return fmt.Sprintf(`<parameter name="%s" type="string" update="set">%s</parameter>`, esc(k), cdata(v))
}

func pref(k, v string) string {
	return fmt.Sprintf(`<preferences name="%s" update="set"><value>%s</value></preferences>`, esc(k), cdata(v))
}

const xmlHead = `<?xml version="1.0" encoding="UTF-8"?>
<request xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xsi:noNamespaceSchemaLocation="PortalConfig_8.5.0.xsd" type="%s"%s>
  <portal action="locate">
`
const xmlTail = `  </portal>
</request>
`

// PageUpdateXML builds the XMLAccess request that creates or updates the page. With withLayout the page gets
// a fresh layout with one Web Content Viewer per Script Application; without it only the titles, parameters
// and access are updated, so the layout (and the settings people made on it) is kept.
func PageUpdateXML(p Page, withLayout bool) string {
	var b strings.Builder
	fmt.Fprintf(&b, xmlHead, "update", ` create-oids="true"`)
	fmt.Fprintf(&b, "    <skin action=\"locate\" objectid=\"skin\" uniquename=\"%s\"/>\n", esc(p.Skin))
	if p.Theme != "" {
		fmt.Fprintf(&b, "    <theme action=\"locate\" objectid=\"theme\" uniquename=\"%s\"/>\n", esc(p.Theme))
	}
	b.WriteString(`    <web-app action="locate" uid="ilwwcm-localrenderingportlet-jsr.war.webmod">
      <portlet-app action="locate" uid="ilwwcm-localrenderingportlet-jsr.war">
        <portlet action="locate" objectid="viewer" name="Web Content Viewer (JSR 286)"/>
      </portlet-app>
    </web-app>
`)
	fmt.Fprintf(&b, "    <content-node action=\"locate\" objectid=\"parent\" uniquename=\"%s\"/>\n", esc(p.Parent))
	ord := ""
	if p.Ordinal != nil {
		ord = fmt.Sprintf(` ordinal="%d"`, *p.Ordinal)
	}
	theme := ""
	if p.Theme != "" {
		theme = ` themeref="theme"`
	}
	fmt.Fprintf(&b, `    <content-node action="update" active="true" allportletsallowed="true" content-parentref="parent" create-type="explicit"
        domain="rel" objectid="page"%s type="staticpage" uniquename="%s"%s>
      <supported-markup markup="html" update="set"/>
`, ord, esc(p.UniqueName), theme)
	locales := make([]string, 0, len(p.Titles))
	for l := range p.Titles {
		locales = append(locales, l)
	}
	sort.Strings(locales)
	for _, l := range locales {
		fmt.Fprintf(&b, "      <localedata locale=\"%s\"><title>%s</title></localedata>\n", esc(l), esc(p.Titles[l]))
	}
	for _, kv := range [][2]string{
		{"com.ibm.portal.bookmarkable", "Yes"},
		{"com.ibm.portal.friendly.name", p.FriendlyName},
		{"com.ibm.portal.layout.template.file.name.html", "layout.html"},
		{"com.ibm.portal.layout.template.markup", "html"},
		{"com.ibm.portal.layout.template.ref", p.LayoutTemplate},
		{"com.ibm.portal.static.page.file.name.html", "layout.html"},
		{HashParameter, p.Hash},
	} {
		b.WriteString("      " + param(kv[0], kv[1]) + "\n")
	}
	anon := map[bool]string{true: "set", false: "remove"}[p.Anonymous]
	auth := map[bool]string{true: "set", false: "remove"}[p.Authenticated || p.Anonymous]
	fmt.Fprintf(&b, `      <access-control externalized="false" owner="undefined" private="false"><role actionset="User" update="set">
        <mapping subjectid="anonymous portal user" subjecttype="user" update="%s"/>
        <mapping subjectid="all authenticated portal users" subjecttype="user_group" update="%s"/>
      </role></access-control>
`, anon, auth)
	if withLayout {
		b.WriteString(`      <component action="update" active="true" domain="rel" ordinal="100" orientation="V" type="container">
        <component action="update" active="true" domain="rel" ordinal="100" orientation="H" type="container">
          ` + param("com.ibm.portal.layoutnode.localname", "ibmHiddenWidgets") + `
          ` + param("css-classes", "ibmDndRow hiddenWidgetsContainer wpthemeCol12of12 wpthemeFull") + `
        </component>
        <component action="update" active="true" domain="rel" ordinal="200" orientation="H" type="container">
          ` + param("com.ibm.portal.layoutnode.localname", "ibmMainContainer") + `
          ` + param("css-classes", "wpthemeLeft wpthemeCol ibmDndColumn wpthemeCol12of12 wpthemeFull wpthemePrimary") + `
`)
		for i, id := range p.ContentIDs {
			fmt.Fprintf(&b, `          <component action="update" active="true" domain="rel" ordinal="%d" type="control" skinref="skin">
            <portletinstance action="update" domain="rel" portletref="viewer">
              %s
              %s
              %s
              %s
              %s
              %s
              %s
              %s
            </portletinstance>
          </component>
`, 100*(i+1), pref("WCM_CONTENT_TYPE", "CONTENT"), pref("WCM_CONTENT_CONTEXT_TYPE", "content"),
				pref("WCM_CONTENT_CONTEXT_IDR", id), pref("WCM_CONTENT_CONTEXT_IS_REL_PATH", "false"),
				pref("WCM_BROADCASTS_TO", "WCM_LINKING_NONE"), pref("WCM_LISTENS_TO", "WCM_LINKING_NONE"),
				pref("WCM_PAGE_TITLE_TYPE", "WCM_PAGE_TITLE_TYPE_DEFAULT"), pref("WCM_PORTLET_TITLE_TYPE", "WCM_PORTLET_TITLE_TYPE_DYN"))
		}
		b.WriteString("        </component>\n      </component>\n")
	}
	b.WriteString("    </content-node>\n")
	b.WriteString(xmlTail)
	return b.String()
}

// PageExportXML builds the request that exports a page by unique name.
func PageExportXML(uniqueName string) string {
	return fmt.Sprintf(xmlHead, "export", "") +
		fmt.Sprintf("    <content-node action=\"export\" uniquename=\"%s\"/>\n", esc(uniqueName)) + xmlTail
}

// PageDeleteXML builds the request that deletes a page by unique name.
func PageDeleteXML(uniqueName string) string {
	return fmt.Sprintf(xmlHead, "update", "") +
		fmt.Sprintf("    <content-node action=\"delete\" uniquename=\"%s\"/>\n", esc(uniqueName)) + xmlTail
}

var (
	statusOK   = regexp.MustCompile(`<status element="all" result="ok"\s*/>`)
	failMsg    = regexp.MustCompile(`(?s)<message[^>]*>(.*?)</message>`)
	paramValue = regexp.MustCompile(`(?s)<parameter name="([^"]+)"[^>]*>(?:<!\[CDATA\[)?(.*?)(?:\]\]>)?</parameter>`)
)

// XMLAccessOK says whether an XMLAccess result is a success, and the first message otherwise.
func XMLAccessOK(result string) (bool, string) {
	if statusOK.MatchString(result) {
		return true, ""
	}
	if m := failMsg.FindStringSubmatch(result); m != nil {
		return false, strings.TrimSpace(m[1])
	}
	return false, "XMLAccess did not report success"
}

// PageExport is what the operator reads from an exported page.
type PageExport struct {
	Exists bool
	Hash   string
}

// ParsePageExport reads an export result: whether the page exists and its dxop.hash parameter. An answer
// that is neither the page nor "not found" (EJPXA0240E) is an error: the operator must not take a temporary
// failure for a deleted page.
func ParsePageExport(result, uniqueName string) (PageExport, error) {
	ok, msg := XMLAccessOK(result)
	if !ok {
		if strings.Contains(result, "EJPXA0240E") {
			return PageExport{}, nil
		}
		return PageExport{}, fmt.Errorf("XMLAccess export of %s failed: %s", uniqueName, msg)
	}
	if !strings.Contains(result, `uniquename="`+uniqueName+`"`) {
		return PageExport{}, nil
	}
	e := PageExport{Exists: true}
	for _, m := range paramValue.FindAllStringSubmatch(result, -1) {
		if m[1] == HashParameter {
			e.Hash = strings.TrimSpace(m[2])
		}
	}
	return e, nil
}
