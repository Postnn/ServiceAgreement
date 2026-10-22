// Package service owns application operations independently of Windows and SQL.
package service

import (
	"context"
	"errors"

	"github.com/Postnn/ServiceAgreement/internal/domain"
	"github.com/Postnn/ServiceAgreement/internal/readmodel"
)

// Repository is the narrow persistence boundary required by the editor.
type Repository interface {
	Get(context.Context, int) (domain.Record, error)
	Insert(context.Context, domain.Record) error
	Update(context.Context, domain.Record, domain.Record) error
	Delete(context.Context, int) error
	List(context.Context, string) (readmodel.ListResult, error)
	ListUnconfigured(context.Context, string) (readmodel.ListResult, error)
	Contracts(context.Context, string, int) ([]domain.Option, error)
	DocumentTypes(context.Context, string, int) ([]domain.Option, error)
}

type Mutation uint8

const (
	None Mutation = iota
	Updated
	Inserted
	Deleted
)

// Mutation is returned only after a successful write. UI refresh policy belongs
// to the caller; service never reloads lists or opens windows.
type Agreements struct{ repo Repository }

func New(repo Repository) Agreements { return Agreements{repo: repo} }
func (s Agreements) Get(ctx context.Context, id int) (domain.Record, error) {
	return s.repo.Get(ctx, id)
}
func (s Agreements) List(ctx context.Context, search string) (readmodel.ListResult, error) {
	return s.repo.List(ctx, search)
}
func (s Agreements) ListUnconfigured(ctx context.Context, search string) (readmodel.ListResult, error) {
	return s.repo.ListUnconfigured(ctx, search)
}
func (s Agreements) Contracts(ctx context.Context, search string, selected int) ([]domain.Option, error) {
	return s.repo.Contracts(ctx, search, selected)
}
func (s Agreements) DocumentTypes(ctx context.Context, search string, selected int) ([]domain.Option, error) {
	return s.repo.DocumentTypes(ctx, search, selected)
}

func (s Agreements) Insert(ctx context.Context, r domain.Record) (Mutation, error) {
	if err := r.Validate(); err != nil {
		return None, err
	}
	if err := s.repo.Insert(ctx, r); err != nil {
		return None, err
	}
	return Inserted, nil
}
func (s Agreements) Update(ctx context.Context, original, updated domain.Record) (Mutation, error) {
	if original.ContractID != updated.ContractID {
		return None, errors.New("ключ договора изменять нельзя")
	}
	if err := updated.Validate(); err != nil {
		return None, err
	}
	if err := s.repo.Update(ctx, original, updated); err != nil {
		return None, err
	}
	return Updated, nil
}
func (s Agreements) Delete(ctx context.Context, id int) (Mutation, error) {
	if id <= 0 {
		return None, errors.New("выберите договор")
	}
	if err := s.repo.Delete(ctx, id); err != nil {
		return None, err
	}
	return Deleted, nil
}
