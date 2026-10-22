//go:build windows

package main

import (
    "reflect"
    "testing"
)

// These tests describe the SQL LIST query budget for an edit, insert,
// and delete after the database write has already succeeded. A configured
// refresh with checkSchema=true would execute an extra list_sql query
// via QueryListSchema (TOP 1) before loading the list itself.
func TestMutationRefreshMakesOnlyNecessaryListQueries(t *testing.T) {
    scenarios:=[]struct {
        name string
        action parameterMutation
        want []string
    }{
        {
            name:"edit existing parameters",
            action:parametersUpdated,
            want:[]string{"with-parameters"},
        },
        {
            name:"insert parameters from main list",
            action:parametersInserted,
            want:[]string{"with-parameters","without-parameters"},
        },
        {
            name:"insert parameters from second list",
            action:parametersInserted,
            want:[]string{"with-parameters","without-parameters"},
        },
        {
            name:"delete parameters",
            action:parametersDeleted,
            want:[]string{"with-parameters","without-parameters"},
        },
    }
    for _,tc:=range scenarios {
        t.Run(tc.name,func(t *testing.T) {
            calls:=[]string{}
            configuredQueries:=0
            schemaQueries:=0
            unconfiguredQueries:=0
            runMutationRefresh(tc.action,
                func(checkSchema bool) {
                    calls=append(calls,"with-parameters")
                    configuredQueries++
                    if checkSchema {
                        // Would execute list_sql once more using TOP 1.
                        schemaQueries++
                    }
                },
                func() {
                    calls=append(calls,"without-parameters")
                    unconfiguredQueries++
                },
            )
            if !reflect.DeepEqual(calls,tc.want) {
                t.Fatalf("SQL list calls=%v, expected %v",calls,tc.want)
            }
            if configuredQueries!=1 || schemaQueries!=0 {
                t.Fatalf("main list is queried %d times + %d extra schema queries; expected 1 + 0",
                    configuredQueries,schemaQueries)
            }
            expectedSecondary:=0
            if tc.action==parametersInserted||tc.action==parametersDeleted {
                expectedSecondary=1
            }
            if unconfiguredQueries!=expectedSecondary {
                t.Fatalf("unconfigured list queries=%d, expected %d",
                    unconfiguredQueries,expectedSecondary)
            }
        })
    }
}

func TestMutationRefreshKeepsManualSchemaChecksSeparate(t *testing.T) {
    // The dispatcher must never ask for structural synchronization,
    // regardless of the action that changed contract parameters.
    for _,kind:=range []parameterMutation{
        parametersUpdated, parametersInserted, parametersDeleted,
    } {
        callbacks:=0
        runMutationRefresh(kind,
            func(checkSchema bool) {
                callbacks++
                if checkSchema {
                    t.Fatalf("unexpected QueryListSchema on mutation %v",kind)
                }
            },
            func() {},
        )
        if callbacks!=1 {
            t.Fatalf("mutation %v refreshed main list %d times",kind,callbacks)
        }
    }
}
