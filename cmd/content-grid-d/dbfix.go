package main

import (
	"bytes"

	dbm "github.com/cosmos/cosmos-db"
)

// presenceDB wraps a cosmos-db DB to correctly handle keys that exist with an
// empty value (len=0). Some backends (notably goleveldb) may return a nil
// slice for an empty value, which makes it indistinguishable from a missing
// key when callers use Get/Has.
//
// IAVL stores use empty values for empty roots (SaveEmptyRoot), so treating
// empty values as missing causes "version does not exist" during historical
// queries.
//
// This wrapper makes Get/Has reflect key existence based on iteration.
type presenceDB struct{ dbm.DB }

func (db presenceDB) keyExists(key []byte) (bool, error) {
	// Iterator range [key, key+0x00) will only include `key` if it exists.
	end := append(append([]byte{}, key...), 0x00)
	it, err := db.DB.Iterator(key, end)
	if err != nil {
		return false, err
	}
	defer it.Close()
	if !it.Valid() {
		return false, nil
	}
	return bytes.Equal(it.Key(), key), it.Error()
}

func (db presenceDB) Has(key []byte) (bool, error) {
	return db.keyExists(key)
}

func (db presenceDB) Get(key []byte) ([]byte, error) {
	bz, err := db.DB.Get(key)
	if err != nil {
		return nil, err
	}
	if bz != nil {
		return bz, nil
	}
	// bz == nil could mean "missing" or "present with empty value".
	exists, err := db.keyExists(key)
	if err != nil {
		return nil, err
	}
	if exists {
		return []byte{}, nil
	}
	return nil, nil
}
