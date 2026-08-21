package main

import (
	"fmt"
	"os"
	"strings"

	controldb "github.com/multigent/multigent/internal/db"
	"github.com/multigent/multigent/internal/store"
	"github.com/multigent/multigent/internal/taskstore"
)

func openControlDB() (controldb.Store, error) {
	return controldb.OpenDefault()
}

func openControlDBForRoot(root string) (controldb.Store, error) {
	if strings.TrimSpace(os.Getenv("MULTIGENT_DATA_DIR")) == "" && strings.TrimSpace(os.Getenv("MULTIGENT_CONTROL_DATA_DIR")) == "" {
		setCLIDataDirForWorkspace(root)
	}
	return controldb.OpenDefault()
}

func openStore(root string) (store.Store, error) {
	db, err := openControlDBForRoot(root)
	if err != nil {
		return nil, err
	}
	return store.NewDB(root, db), nil
}

func openTaskStore(root string) (taskstore.Store, error) {
	db, err := openControlDBForRoot(root)
	if err != nil {
		return nil, err
	}
	return taskstore.NewDB(root, db), nil
}

func mustStore(root string) store.Store {
	s, err := openStore(root)
	if err != nil {
		panic(fmt.Sprintf("open store: %v", err))
	}
	return s
}

func mustTaskStore(root string) taskstore.Store {
	s, err := openTaskStore(root)
	if err != nil {
		panic(fmt.Sprintf("open task store: %v", err))
	}
	return s
}
