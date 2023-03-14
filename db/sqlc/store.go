package db

import (
	"context"
	"database/sql"
	"fmt"
)

// Provides all functions to execute db queries and transactions.
type Store struct {
	*Queries
	db *sql.DB
}

func NewStore(db *sql.DB) *Store {
	return &Store{
		db:      db,
		Queries: New(db),
	}
}

// Executes a function within a database transaction.
func (store *Store) execTx(ctx context.Context, fn func(*Queries) error) error {
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}

	q := New(tx)
	if err := fn(q); err != nil {
		if rbErr := tx.Rollback(); rbErr != nil {
			return fmt.Errorf("tx err: %v, rollback err: %v", err, rbErr)
		}
		return err
	}

	return tx.Commit()
}

// Contains the input parameter of transfer transaction.
type TransferTxParams struct {
	FromAccountID int64 `json:"from_account_id"`
	ToAccountID   int64 `json:"to_account_id"`
	Amount        int64 `json:"amount"`
}

// The result of the transfer transaction.
type TransferTxResult struct {
	Transfer    Transfer `json:"transfer"`
	FromAccount Account  `json:"from_account"`
	ToAccount   Account  `json:"to_account"`
	FromEntry   Entry    `json:"from_entry"`
	ToEntry     Entry    `json:"to_entry"`
}

/**
 * Performs a money transfer from one account to the other.
 * It creates a transfer record, add account entries,
 * and update account's balance within a single database transaction.
 */
func (store *Store) TransferTx(ctx context.Context, arg TransferTxParams) (
	TransferTxResult, error,
) {
	var result TransferTxResult

	err := store.execTx(ctx, func(q *Queries) error {
		var err error
		result.Transfer, err = q.CreateTransfer(ctx, CreateTransferParams(arg))
		if err != nil {
			return err
		}

		result.FromEntry, err = q.CreateEntry(ctx, CreateEntryParams{
			AccountID: arg.FromAccountID,
			Amount:    -arg.Amount,
		})
		if err != nil {
			return err
		}

		result.ToEntry, err = q.CreateEntry(ctx, CreateEntryParams{
			AccountID: arg.ToAccountID,
			Amount:    arg.Amount,
		})
		if err != nil {
			return err
		}

		// move money out of account1
		account1, err := q.GetAccountForUpdate(ctx, arg.FromAccountID)
		if err != nil {
			return err
		}

		result.FromAccount, err = q.UpdateAccount(ctx, UpdateAccountParams{
			ID:      arg.FromAccountID,
			Balance: account1.Balance - arg.Amount,
		})
		if err != nil {
			return err
		}

		// move money into account2
		account2, err := q.GetAccountForUpdate(ctx, arg.ToAccountID)
		if err != nil {
			return err
		}

		result.ToAccount, err = q.UpdateAccount(ctx, UpdateAccountParams{
			ID:      arg.ToAccountID,
			Balance: account2.Balance + arg.Amount,
		})
		if err != nil {
			return err
		}

		return nil
	})

	return result, err
}
