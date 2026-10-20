package core

import "testing"

func TestMoneyDisplayN2RussianFormat(t *testing.T) {
    cases:=[]struct{value any; want string}{
        {"0.0000","0,00"},
        {"1234.5000","1\u00a0234,50"},
        {"-1234567890.1260","-1\u00a0234\u00a0567\u00a0890,13"},
        {"1.0050","1,00"},
        {"1.0150","1,02"},
        {"999.9950","1\u00a0000,00"},
        {[]byte("30000.8"),"30\u00a0000,80"},
        {int64(42),"42,00"},
        {nil,""},
    }
    for _,c:=range cases {
        got,ok:=MoneyDisplayN2(c.value)
        if !ok || got!=c.want {
            t.Errorf("%#v => %q, %v; want %q",c.value,got,ok,c.want)
        }
    }
}
func TestSQLDisplayValueFormatsMoneyBySQLType(t *testing.T) {
    for _,typ:=range []string{"MONEY","SMALLMONEY","money"} {
        if got:=SQLDisplayValue("4567.2500",typ,"ServiceFee");got!="4\u00a0567,25" {
            t.Errorf("%s: %q",typ,got)
        }
    }
    if got:=SQLDisplayValue("1000.2500","NVARCHAR","Amount");got!="1\u00a0000,25" {
        t.Fatalf("backward compatible Amount display: %s",got)
    }
    if got:=SQLDisplayValue("1000.2500","NVARCHAR","ContractName");got!="1000.2500" {
        t.Fatalf("non-money text changed: %s",got)
    }
}
