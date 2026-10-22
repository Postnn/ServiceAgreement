package service

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/Postnn/ServiceAgreement/internal/domain"
	"github.com/Postnn/ServiceAgreement/internal/readmodel"
)

type repositorySpy struct {
	calls             []string
	err               error
	original, written domain.Record
	ctx               context.Context
}

func (r *repositorySpy) call(ctx context.Context, name string) error {
	r.ctx = ctx
	r.calls = append(r.calls, name)
	return r.err
}
func (r *repositorySpy) Get(c context.Context, id int) (domain.Record, error) {
	return r.original, r.call(c, "get")
}
func (r *repositorySpy) Insert(c context.Context, v domain.Record) error {
	r.written = v
	return r.call(c, "insert")
}
func (r *repositorySpy) Update(c context.Context, old, v domain.Record) error {
	r.original = old
	r.written = v
	return r.call(c, "update")
}
func (r *repositorySpy) Delete(c context.Context, id int) error { return r.call(c, "delete") }
func (r *repositorySpy) List(c context.Context, q string) (readmodel.ListResult, error) {
	return readmodel.ListResult{}, r.call(c, "list")
}
func (r *repositorySpy) ListUnconfigured(c context.Context, q string) (readmodel.ListResult, error) {
	return readmodel.ListResult{}, r.call(c, "unconfigured")
}
func (r *repositorySpy) Contracts(c context.Context, q string, id int) ([]domain.Option, error) {
	return nil, r.call(c, "contracts")
}
func (r *repositorySpy) DocumentTypes(c context.Context, q string, id int) ([]domain.Option, error) {
	return nil, r.call(c, "types")
}
func validRecord() domain.Record {
	return domain.Record{ContractID: 1, TypeID: 2, Period: 3, Amount: "123,4567", StartDay: 7, StartMonth: 1, DocumentsCode: -6, PaymentCode: 306, DisconnectCode: 320}
}

func TestMutationsReportSuccessOnlyAfterWriteAndNeverReloadLists(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	for _, action := range []string{"insert", "update", "delete"} {
		for _, writeErr := range []error{nil, domain.ErrConflict, context.Canceled} {
			r := &repositorySpy{err: writeErr}
			s := New(r)
			v := validRecord()
			original := v
			original.Amount = "123.4567"
			var got Mutation
			var err error
			var want Mutation
			switch action {
			case "insert":
				got, err = s.Insert(ctx, v)
				want = Inserted
			case "update":
				got, err = s.Update(ctx, original, v)
				want = Updated
			case "delete":
				got, err = s.Delete(ctx, 1)
				want = Deleted
			}
			if writeErr != nil {
				want = None
			}
			if got != want || !errors.Is(err, writeErr) {
				t.Fatalf("%s: mutation=%v err=%v", action, got, err)
			}
			if !reflect.DeepEqual(r.calls, []string{action}) || r.ctx != ctx {
				t.Fatalf("%s: unexpected calls/context: %v", action, r.calls)
			}
			if action != "delete" && r.written.Amount != "123.4567" {
				t.Fatal("normalized amount not passed to repository")
			}
			if action == "update" && r.original != original {
				t.Fatal("concurrency snapshot changed")
			}
		}
	}
}
func TestInvalidMutationDoesNotReachRepository(t *testing.T) {
	r := &repositorySpy{}
	s := New(r)
	ctx := context.Background()
	valid := validRecord()
	bad := valid
	bad.PaymentCode = 0
	for _, op := range []func() (Mutation, error){
		func() (Mutation, error) { return s.Insert(ctx, bad) },
		func() (Mutation, error) { return s.Update(ctx, valid, bad) },
		func() (Mutation, error) {
			changed := valid
			changed.ContractID = 2
			return s.Update(ctx, valid, changed)
		},
		func() (Mutation, error) { return s.Delete(ctx, 0) },
	} {
		if mutation, err := op(); err == nil || mutation != None {
			t.Fatalf("invalid operation: %v %v", mutation, err)
		}
	}
	if len(r.calls) != 0 {
		t.Fatalf("invalid commands wrote data: %v", r.calls)
	}
}
