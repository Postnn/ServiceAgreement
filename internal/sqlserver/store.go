package sqlserver

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/Postnn/ServiceAgreement/internal/config"
	"github.com/Postnn/ServiceAgreement/internal/domain"
)

type Store struct {
	DB     *sql.DB
	Config config.Config
}

func (s Store) Lookup(ctx context.Context, query, search string, selectedID int) ([]domain.Option, error) {
	rows, err := s.DB.QueryContext(ctx, query, sql.Named("search", strings.TrimSpace(search)), sql.Named("limit", s.Config.LookupLimit), sql.Named("selected", selectedID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]domain.Option, 0)
	for rows.Next() {
		var opt domain.Option
		if err := rows.Scan(&opt.ID, &opt.Label); err != nil {
			return nil, fmt.Errorf("столбцы справочника: %w", err)
		}
		result = append(result, opt)
	}
	return result, rows.Err()
}

func (s Store) Get(ctx context.Context, id int) (domain.Record, error) {
	var r domain.Record
	err := s.DB.QueryRowContext(ctx, s.Config.GetSQL, sql.Named("id", id)).Scan(
		&r.ContractID, &r.TypeID, &r.Period, &r.AutoPay, &r.Amount,
		&r.StartDay, &r.StartMonth, &r.DocumentsCode, &r.PaymentCode, &r.DisconnectCode, &r.Notify)
	return r, err
}

func recordArgs(r domain.Record) []interface{} {
	return []interface{}{sql.Named("id", r.ContractID), sql.Named("type", r.TypeID), sql.Named("period", r.Period),
		sql.Named("auto", r.AutoPay), sql.Named("amount", r.Amount), sql.Named("day", r.StartDay),
		sql.Named("month", r.StartMonth), sql.Named("docs", r.DocumentsCode), sql.Named("pay", r.PaymentCode),
		sql.Named("off", r.DisconnectCode), sql.Named("notify", r.Notify)}
}
func (s Store) Insert(ctx context.Context, r domain.Record) error {
	if err := r.Validate(); err != nil {
		return err
	}
	_, err := s.DB.ExecContext(ctx, s.Config.InsertSQL, recordArgs(r)...)
	return err
}

func (s Store) Update(ctx context.Context, original, updated domain.Record) error {
	if updated.ContractID != original.ContractID {
		return errors.New("ключ договора изменять нельзя")
	}
	if err := updated.Validate(); err != nil {
		return err
	}
	// Preserve exact original SQL money scale for optimistic concurrency checks.
	oldAmount, err := domain.NormalizeMoney(original.Amount)
	if err != nil {
		return err
	}
	args := append(recordArgs(updated),
		sql.Named("old_type", original.TypeID), sql.Named("old_period", original.Period),
		sql.Named("old_auto", original.AutoPay), sql.Named("old_amount", oldAmount),
		sql.Named("old_day", original.StartDay), sql.Named("old_month", original.StartMonth),
		sql.Named("old_docs", original.DocumentsCode), sql.Named("old_pay", original.PaymentCode),
		sql.Named("old_off", original.DisconnectCode), sql.Named("old_notify", original.Notify))
	res, err := s.DB.ExecContext(ctx, s.Config.UpdateSQL, args...)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected != 1 {
		return domain.ErrConflict
	}
	return nil
}

func (s Store) Delete(ctx context.Context, id int) error {
	res, err := s.DB.ExecContext(ctx, s.Config.DeleteSQL, sql.Named("id", id))
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected != 1 {
		return domain.ErrConflict
	}
	return nil
}

func (s Store) Contracts(ctx context.Context, search string, selected int) ([]domain.Option, error) {
	return s.Lookup(ctx, s.Config.ContractsSQL, search, selected)
}
func (s Store) DocumentTypes(ctx context.Context, search string, selected int) ([]domain.Option, error) {
	return s.Lookup(ctx, s.Config.DocumentTypesSQL, search, selected)
}
