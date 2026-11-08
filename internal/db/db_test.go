package db

import (
	"regexp"
	"strings"
	"testing"
)

func TestPasswordFitsEveryEngine(t *testing.T) {
	for i := 0; i < 200; i++ {
		p := Password()
		if len(p) > 30 || !regexp.MustCompile(`[A-Z]`).MatchString(p) || !regexp.MustCompile(`[a-z]`).MatchString(p) ||
			!regexp.MustCompile(`[0-9]`).MatchString(p) || strings.ContainsAny(p, `'"-_`) {
			t.Fatalf("bad password %q", p)
		}
	}
}

func TestNormaliseDefaults(t *testing.T) {
	pg := Target{Host: "pg", Database: "visits"}
	if err := pg.Normalise(); err != nil || pg.Engine != PostgreSQL || pg.Port != 5432 || pg.Owner != "visits" || pg.SSLMode != "prefer" {
		t.Fatalf("postgres: %+v %v", pg, err)
	}
	ora := Target{Engine: Oracle, Host: "ora", Database: "visits", Owner: "other", Service: "FREEPDB1"}
	if err := ora.Normalise(); err != nil || ora.Port != 1521 || ora.Owner != "visits" || ora.Tablespace != "USERS" {
		t.Fatalf("oracle: %+v %v", ora, err)
	}
	if err := (&Target{Engine: Oracle, Host: "ora", Database: "visits"}).Normalise(); err == nil {
		t.Fatal("oracle without a service must fail")
	}
	ms := Target{Engine: SQLServer, Host: "ms", Database: "Visits"}
	if err := ms.Normalise(); err != nil || ms.Port != 1433 || ms.Encrypt != "true" {
		t.Fatalf("sqlserver: %+v %v", ms, err)
	}
	for _, bad := range []Target{{Host: "pg", Database: "Bad-Name"}, {Engine: SQLServer, Host: "ms", Database: "a];DROP"}, {Engine: "DB2", Host: "x", Database: "a"}} {
		if err := bad.Normalise(); err == nil {
			t.Errorf("%+v must be rejected", bad)
		}
	}
}

func TestDetails(t *testing.T) {
	cases := []struct {
		t    Target
		jdbc string
		uri  string
	}{
		{Target{Engine: PostgreSQL, Host: "pg", Database: "visits"}, "jdbc:postgresql://pg:5432/visits?sslmode=prefer", "postgresql://visits:pw@pg:5432/visits?sslmode=prefer"},
		{Target{Engine: Oracle, Host: "ora", Database: "visits", Service: "FREEPDB1"}, "jdbc:oracle:thin:@//ora:1521/FREEPDB1", "oracle://VISITS:pw@ora:1521/FREEPDB1"},
		{Target{Engine: SQLServer, Host: "ms", Database: "visits", TrustCert: true}, "jdbc:sqlserver://ms:1433;databaseName=visits;encrypt=true;trustServerCertificate=true", "sqlserver://visits:pw@ms:1433?TrustServerCertificate=true&database=visits&encrypt=true"},
	}
	for _, c := range cases {
		if err := c.t.Normalise(); err != nil {
			t.Fatal(err)
		}
		d := c.t.Details("pw")
		if d["jdbcUrl"] != c.jdbc || d["uri"] != c.uri || d["engine"] != c.t.Engine || d["password"] != "pw" {
			t.Errorf("%s: %v", c.t.Engine, d)
		}
	}
}

func TestStatements(t *testing.T) {
	o := Target{Engine: Oracle, Host: "ora", Database: "visits", Service: "FREEPDB1"}
	_ = o.Normalise()
	s := strings.Join(o.oracleStatements(false, "Pw1"), "\n")
	for _, want := range []string{`CREATE USER VISITS IDENTIFIED BY "Pw1" DEFAULT TABLESPACE USERS QUOTA UNLIMITED ON USERS`, "GRANT CREATE SESSION"} {
		if !strings.Contains(s, want) {
			t.Errorf("oracle: missing %q in %s", want, s)
		}
	}
	if s := strings.Join(o.oracleStatements(true, "Pw1"), ""); !strings.HasPrefix(s, `ALTER USER VISITS IDENTIFIED BY "Pw1"`) {
		t.Errorf("oracle existing user: %s", s)
	}
	m := Target{Engine: SQLServer, Host: "ms", Database: "visits"}
	_ = m.Normalise()
	s = strings.Join(m.sqlServerStatements("Pw'1"), "\n")
	for _, want := range []string{"CREATE LOGIN [visits] WITH PASSWORD = N'Pw''1'", "IF DB_ID(N'visits') IS NULL CREATE DATABASE [visits]", "USE [visits];", "ALTER ROLE db_owner ADD MEMBER [visits]"} {
		if !strings.Contains(s, want) {
			t.Errorf("sqlserver: missing %q in %s", want, s)
		}
	}
}
