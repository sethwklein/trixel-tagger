package main

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"sethwklein.net/fttp"

	"github.com/boltdb/bolt"
)

// trixel -> tags
// tag -> trixels

// trids before tags

// both buckets use keys only
// keys consisting of the trixel id and the tag
// one has the trixel first in the key
// the other has the tag first

// i think i can use go quoted string format for the tag
// check id's for /^[0-9]+$/

var db *bolt.DB
var trid2tag = []byte("trid2tag")
var tag2trid = []byte("tag2trid")
var tagCounts = []byte("tagCounts")

func initDB() error {
	var err error

	db, err = bolt.Open("trixel-tags.bolt", 0666, nil)
	if err != nil {
		return err
	}
	return db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists(tag2trid)
		if err != nil {
			return err
		}
		_, err = tx.CreateBucketIfNotExists(trid2tag)
		if err != nil {
			return err
		}
		_, err = tx.CreateBucketIfNotExists(tagCounts)
		if err != nil {
			return err
		}
		return nil
	})
}

// keys returns the keys that associate a trid with a tag.
func keys(trid string, tag string) (tridKey, tagKey []byte) {
	quoted := strconv.AppendQuote(nil, tag)

	tridKey = make([]byte, len(trid)+len(quoted))
	copy(tridKey, trid)
	copy(tridKey[len(trid):], quoted)

	tagKey = make([]byte, len(quoted)+len(trid))
	copy(tagKey, quoted)
	copy(tagKey[len(quoted):], trid)

	return tridKey, tagKey
}

func deleteTag(trid string, tag string) error {
	tridKey, tagKey := keys(trid, tag)
	tagBuf := []byte(tag)

	return db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(trid2tag)
		err := b.Delete(tridKey)
		if err != nil {
			return err
		}

		b = tx.Bucket(tag2trid)
		err = b.Delete(tagKey)
		if err != nil {
			return err
		}

		// watch for the early return
		b = tx.Bucket(tagCounts)
		var count int32
		{
			buf := b.Get(tagBuf)
			err = binary.Read(bytes.NewBuffer(buf), binary.BigEndian,
				&count)
			if err != nil {
				return err
			}
		}
		if count == 1 {
			return b.Delete(tagBuf)
		}
		count--
		{
			buf := &bytes.Buffer{}
			err = binary.Write(buf, binary.BigEndian, count)
			if err != nil {
				return err
			}
			return b.Put(tagBuf, buf.Bytes())
		}
	})
}

func putTag(trid string, tag string) error {
	tridKey, tagKey := keys(trid, tag)
	tagBuf := []byte(tag)

	return db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(trid2tag)
		err := b.Put(tridKey, nil)
		if err != nil {
			return err
		}

		b = tx.Bucket(tag2trid)
		err = b.Put(tagKey, nil)
		if err != nil {
			return err
		}

		b = tx.Bucket(tagCounts)
		count := int32(0)
		{
			buf := b.Get(tagBuf)
			if buf != nil {
				err = binary.Read(bytes.NewBuffer(buf),
					binary.BigEndian, &count)
				if err != nil {
					return err
				}
			}
		}
		count++
		{
			buf := &bytes.Buffer{}
			err = binary.Write(buf, binary.BigEndian, count)
			if err != nil {
				return err
			}
			return b.Put(tagBuf, buf.Bytes())
		}
	})
}

func getTrids(tag string) ([]string, error) {
	quoted := strconv.AppendQuote(nil, tag)

	trids := []string{}
	err := db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(tag2trid)
		c := b.Cursor()
		for k, _ := c.Seek(quoted); k != nil; k, _ = c.Next() {
			if !bytes.HasPrefix(k, quoted) {
				break
			}
			trids = append(trids, string(k[len(quoted):]))
		}
		return nil
	})
	return trids, err
}

func getTags(trid string) ([]string, error) {
	buf := make([]byte, len(trid)+1)
	copy(buf, trid)
	buf[len(buf)-1] = '"'

	tags := []string{}
	err := db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(trid2tag)
		c := b.Cursor()
		for k, _ := c.Seek(buf); k != nil; k, _ = c.Next() {
			if !bytes.HasPrefix(k, buf) {
				break
			}
			quoted := string(k[len(trid):])
			tag, err := strconv.Unquote(quoted)
			if err != nil {
				return err
			}
			tags = append(tags, tag)
		}
		return nil
	})
	return tags, err
}

func listTags() ([]string, error) {
	tags := []string{}
	err := db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(tagCounts)
		c := b.Cursor()
		for k, _ := c.First(); k != nil; k, _ = c.Next() {
			tags = append(tags, string(k))
		}
		return nil
	})
	return tags, err
}

// GET /trids/{trid} -> tags
// DELETE /trids/{trid}/tag/{tag}
//   only this way because encoding

// GET /tags/ -> tags
// GET /tags/{tag} -> trids
// POST /tags/{tag} body:trid -> 204

// check trid's for /^[0-9]+$/

// BUG: json errors
// BUG: 204's

type NotTridError string

func (err NotTridError) Error() string {
	return fmt.Sprintf("not a Trixel ID: %s", string(err))
}

func checkTrid(q *http.Request, maybeTrid string) (string, fttp.Responder) {
	for i := 0; i < len(maybeTrid); i++ {
		c := maybeTrid[i]
		if c < '0' || c > '9' {
			return "", fttp.NotFound(q, NotTridError(maybeTrid))
		}
	}
	return maybeTrid, nil
}

var tridPath = "/trids/"

func init() {
	fttp.Handle(tridPath, considerTrids)
}
func considerTrids(q *http.Request) fttp.Responder {
	switch q.Method {
	case "GET":
		// let the range check panic indicate the problem
		trid, rx := checkTrid(q, q.URL.Path[len(tridPath):])
		if rx != nil {
			return rx
		}

		tags, err := getTags(trid)
		if err != nil {
			return fttp.ServerError(err)
		}

		jason, err := json.Marshal(tags)
		if err != nil {
			return fttp.ServerError(err)
		}

		r := &fttp.Response{}
		r.JSON()
		r.Set(jason)
		return r
	case "DELETE":
		// let the range check panic indicate the problem
		suffix := q.URL.Path[len(tridPath):]
		sep := strings.IndexByte(suffix, '/')
		if sep < 1 {
			// BUG: bad request
			return fttp.NotFound(q, errors.New("trid or tag not specified"))
		}
		trid, rx := checkTrid(q, suffix[:sep])
		if rx != nil {
			return rx
		}
		suffix = suffix[sep:]
		if !strings.HasPrefix(suffix, "/tag/") {
			// BUG: bad request
			return fttp.NotFound(q, errors.New("tag not specified"))
		}
		tag := suffix[len("/tag/"):]

		err := deleteTag(trid, tag)
		if err != nil {
			return fttp.ServerError(err)
		}

		r := &fttp.Response{}
		r.JSON()
		return r
	default:
		return fttp.BadMethod(q, "GET", "DELETE")
	}
}

var tagPath = "/tags/"

func init() {
	fttp.Handle(tagPath, considerTags)
}
func considerTags(q *http.Request) fttp.Responder {
	// let the range check panic indicate the problem
	tag := q.URL.Path[len(tagPath):]

	switch q.Method {
	case "GET":
		if len(tag) == 0 {
			tags, err := listTags()
			if err != nil {
				return fttp.ServerError(err)
			}

			jason, err := json.Marshal(tags)
			if err != nil {
				return fttp.ServerError(err)
			}

			r := &fttp.Response{}
			r.JSON()
			r.Set(jason)
			return r
		} else {
			trids, err := getTrids(tag)
			if err != nil {
				return fttp.ServerError(err)
			}

			jason, err := json.Marshal(trids)
			if err != nil {
				return fttp.ServerError(err)
			}

			r := &fttp.Response{}
			r.JSON()
			r.Set(jason)
			return r
		}
	case "POST":
		// BUG: limit size. perhaps see http.MaxBytesReader
		trid, err := ioutil.ReadAll(q.Body)
		if err != nil {
			return fttp.ServerError(err)
		}

		err = putTag(string(trid), tag)
		if err != nil {
			return fttp.ServerError(err)
		}

		// BUG: 204
		r := &fttp.Response{}
		r.JSON()
		return r
	default:
		return fttp.BadMethod(q, "GET", "POST", "DELETE")
	}
}

func mainError() (err error) {
	err = initDB()
	if err != nil {
		return err
	}

	http.Handle("/", http.FileServer(http.Dir(".")))

	address := ":8080"
	log.Println("Listening on", address)
	return http.ListenAndServe(address, nil)
}

func mainCode() int {
	err := mainError()
	if err == nil {
		return 0
	}
	fmt.Fprintf(os.Stderr, "%v: Error: %v\n", filepath.Base(os.Args[0]), err)
	return 1
}

func main() {
	os.Exit(mainCode())
}
