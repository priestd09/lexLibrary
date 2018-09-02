// Copyright (c) 2017-2018 Townsourced Inc.

package app

import (
	"database/sql"
	"strings"
	"time"

	"github.com/lexLibrary/lexLibrary/data"
	"github.com/microcosm-cc/bluemonday"
)

// Document is an instance of a published document
type Document struct {
	DocumentContent

	Created       time.Time `json:"created,omitempty"`
	creator       data.ID
	groups        []data.ID
	publishGroups []data.ID

	accessor *User
}

// DocumentContent is the contents of a document who's structure is shared between drafts, history records, and
// published documents
// There can be multiple language versions of the same document
type DocumentContent struct {
	ID       data.ID   `json:"id"`
	Version  int       `json:"version"`
	Language Language  `json:"language"`
	Title    string    `json:"title"`
	Content  string    `json:"content"`
	Tags     []Tag     `json:"tags"`
	Created  time.Time `json:"created,omitempty"`
	creator  data.ID
	Updated  time.Time `json:"updated,omitempty"`
	updater  data.ID
}

const (
	tagTypeUser = "user"
	tagTypeAuto = "auto"
)

// Tag is a string value that
type Tag struct {
	Value    string `json:"value"`
	Type     string `json:"type"`
	Language Language
	Stem     string `json:"stem"`
}

// DocumentDraft is a draft of a document, not visible to the public
type DocumentDraft struct {
	ID data.ID `json:"id"`
	DocumentContent

	editor *User // current user editing the draft
}

// DocumentHistory is a previously published version of a document
type DocumentHistory struct {
	DocumentContent
}

var (
	errDocumentConflict = Conflict("You are not editing the most current version of this document. " +
		"Please refresh and try again")
	errDocumentUpdateAccess = Unauthorized("You do not have access to update this document")
	errDocumentReadAccess   = Unauthorized("You do not have access to view this document")
	errDocumentNotFound     = NotFound("Document not found")
)

var (
	sanitizePolicy = bluemonday.UGCPolicy()
)

// DocumentGet retrieves a document
func DocumentGet(id data.ID, lan Language, who *User) (*Document, error) {
	if id.IsNil() {
		return nil, errDocumentNotFound
	}

	d, err := documentGet(id, lan)
	if err != nil {
		return nil, err
	}

	err = d.tryAccess(who)
	if err != nil {
		return nil, err
	}

	return d, nil
}

func documentGet(id data.ID, lan Language) (*Document, error) {
	d := &Document{}

	rows, err := sqlDocument.get.Query(data.Arg("id", id), data.Arg("language", lan))
	if err != nil {
		return nil, err
	}

	defer rows.Close()

	found := false

	for rows.Next() {
		found = true
		var tag, tagType, stem sql.NullString
		var tagLan Language
		var groupID data.ID
		var canPublish sql.NullBool

		err := rows.Scan(
			&d.ID,
			&d.Created,
			&d.creator,
			&d.DocumentContent.Language,
			&d.DocumentContent.Title,
			&d.DocumentContent.Content,
			&d.DocumentContent.Version,
			&d.DocumentContent.Updated,
			&d.DocumentContent.Created,
			&d.DocumentContent.creator,
			&d.DocumentContent.updater,
			&tag,
			&tagLan,
			&tagType,
			&stem,
			&groupID,
			&canPublish,
		)

		if err != nil {
			return nil, err
		}

		if tag.Valid && tagType.Valid && stem.Valid {
			d.Tags = append(d.Tags, Tag{
				Value:    tag.String,
				Language: tagLan,
				Type:     tagType.String,
				Stem:     stem.String,
			})
		}

		if !groupID.IsNil() {
			d.groups = append(d.groups, groupID)
			if canPublish.Valid && canPublish.Bool {
				d.publishGroups = append(d.publishGroups, groupID)
			}
		}
	}

	if !found {
		return nil, errDocumentNotFound
	}

	return d, nil
}

// tryAccess tries to access the document with the given user
func (d *Document) tryAccess(who *User) error {
	if len(d.groups) == 0 {
		if who == nil && !SettingMust("AllowPublicDocuments").Bool() {
			return errDocumentNotFound
		}
		d.accessor = who
		return nil
	}
	if who == nil {
		return errDocumentNotFound
	}

	if who.IsAdmin() {
		d.accessor = who
		return nil
	}

	count := 0
	err := sqlUser.isGroupMember.QueryRow(
		append(data.Args("group_id", d.groups),
			data.Arg("user_id", who.ID))...,
	).Scan(&count)

	if err != nil {
		return err
	}

	if count == 0 {
		return errDocumentReadAccess
	}

	d.accessor = who
	return nil
}

// Draft retrieves a document draft
func (u *User) Draft(id data.ID) (*DocumentDraft, error) {
	if id.IsNil() {
		return nil, errDocumentNotFound
	}

	d, err := draftGet(id)
	if err != nil {
		return nil, err
	}

	err = d.tryAccess(u)
	if err != nil {
		return nil, err
	}

	return d, nil
}

// tryAccess tries to access the document draft with the given user
func (d *DocumentDraft) tryAccess(who *User) error {
	d.editor = who
	// if they have access to publish, then they can access
	// any draft of the document they can publish on
	return d.canPublish()
}

func draftGet(id data.ID) (*DocumentDraft, error) {
	d := &DocumentDraft{}

	rows, err := sqlDocument.getDraft.Query(data.Arg("id", id))
	if err != nil {
		return nil, err
	}

	defer rows.Close()

	found := false

	for rows.Next() {
		found = true
		var tag, tagType, stem sql.NullString
		var tagLan Language

		err := rows.Scan(
			&d.ID,
			&d.DocumentContent.ID,
			&d.DocumentContent.Language,
			&d.DocumentContent.Title,
			&d.DocumentContent.Content,
			&d.DocumentContent.Version,
			&d.DocumentContent.Updated,
			&d.DocumentContent.Created,
			&d.DocumentContent.creator,
			&d.DocumentContent.updater,
			&tag,
			&tagLan,
			&tagType,
			&stem,
		)

		if err != nil {
			return nil, err
		}

		if tag.Valid && tagType.Valid && stem.Valid {
			d.Tags = append(d.Tags, Tag{
				Value:    tag.String,
				Language: tagLan,
				Type:     tagType.String,
				Stem:     stem.String,
			})
		}
	}

	if !found {
		return nil, errDocumentNotFound
	}

	return d, nil
}

// NewDocument starts a new document and returns the draft of it
func (u *User) NewDocument(title string, lan Language) (*DocumentDraft, error) {
	d := &DocumentDraft{
		ID:     data.NewID(),
		editor: u,
		DocumentContent: DocumentContent{
			Title:    title,
			Language: lan,
			Version:  0,
			Updated:  time.Now(),
			Created:  time.Now(),
			creator:  u.ID,
			updater:  u.ID,
		},
	}
	err := d.validate()
	if err != nil {
		return nil, err
	}

	err = data.BeginTx(func(tx *sql.Tx) error {
		return d.insert(tx)
	})

	if err != nil {
		return nil, err
	}

	return d, nil
}

// NewDraft creates a new Draft for the given document
func (d *Document) NewDraft(lan Language) (*DocumentDraft, error) {
	if d.accessor == nil {
		return nil, errDocumentUpdateAccess
	}

	draft := &DocumentDraft{
		ID:     data.NewID(),
		editor: d.accessor,
		DocumentContent: DocumentContent{
			ID:       d.ID,
			Language: lan,
			Title:    d.DocumentContent.Title,
			Content:  d.DocumentContent.Content,
			Tags:     make([]Tag, 0, len(d.Tags)),
			Version:  0,
			Updated:  time.Now(),
			Created:  time.Now(),
			creator:  d.accessor.ID,
			updater:  d.accessor.ID,
		},
	}

	for i := range d.Tags {
		draft.addTag(d.Tags[i].Type, d.Tags[i].Value)
	}

	err := data.BeginTx(func(tx *sql.Tx) error {
		return draft.insert(tx)
	})

	if err != nil {
		return nil, err
	}

	return draft, nil
}

func (d *DocumentContent) validate() error {
	if strings.TrimSpace(d.Title) == "" {
		return NewFailure("A title is required on documents")
	}

	err := data.FieldValidate("document.language", d.Language.String())
	if err != nil {
		return err
	}

	for i := range d.Tags {
		err = data.FieldValidate("document.tag", d.Tags[i].Value)
		if err != nil {
			return NewFailureFromErr(err)
		}
	}

	return nil
}

// sanitize removes any unneeded, unsupported, or unsafe content
func (d *DocumentContent) sanitize() {
	d.Content = sanitizePolicy.Sanitize(d.Content)
}

// autoTag builds tags automatically based on the document's content. User specified tags
// have a greater weight than auto generated tags
func (d *DocumentContent) autoTag() error {
	return nil
}

// addTag adds a tag to the given document, including the tag's stem.  It won't add the tag if one already exists
func (d *DocumentContent) addTag(tagType, tagValue string) {
	for i := range d.Tags {
		if d.Tags[i].Value == tagValue && d.Tags[i].Type != tagTypeAuto {
			return
		}
	}

	tag := Tag{
		Language: d.Language,
		Value:    tagValue,
		Type:     tagType,
	}

	tag.stem()

	d.Tags = append(d.Tags, tag)
}

func (t *Tag) stem() {
	t.Language.Stem(t.Value)
}

func (d *DocumentDraft) insert(tx *sql.Tx) error {
	if tx == nil {
		panic("A transaction is required when inserting a document draft")
	}

	_, err := sqlDocument.insertDraft.Tx(tx).Exec(
		data.Arg("id", d.ID),
		data.Arg("document_id", d.DocumentContent.ID),
		data.Arg("language", d.Language),
		data.Arg("title", d.Title),
		data.Arg("content", d.Content),
		data.Arg("version", d.Version),
		data.Arg("updated", d.Updated),
		data.Arg("created", d.Created),
		data.Arg("creator", d.creator),
		data.Arg("updater", d.updater),
	)
	if err != nil {
		return err
	}

	for i := range d.Tags {
		_, err = sqlDocument.insertDraftTag.Tx(tx).Exec(
			data.Arg("draft_id", d.ID),
			data.Arg("language", d.Tags[i].Language),
			data.Arg("tag", d.Tags[i].Value),
			data.Arg("stem", d.Tags[i].Stem),
			data.Arg("type", d.Tags[i].Type),
		)
		if err != nil {
			return err
		}
	}
	return nil
}

// Save saves the current document draft
func (d *DocumentDraft) Save(title, content string, tags []string, version int) error {
	if d.editor == nil || d.editor.ID != d.creator {
		return errDocumentUpdateAccess
	}
	//TODO: Invite others to work on your draft

	return data.BeginTx(func(tx *sql.Tx) error {
		return d.update(tx, title, content, tags, version)
	})
}

func (d *DocumentDraft) update(tx *sql.Tx, title, content string, tags []string, version int) error {
	d.Title = title
	d.Content = content
	d.Version = version
	d.Tags = nil

	for i := range tags {
		d.addTag(tagTypeUser, tags[i])
	}

	err := d.validate()
	if err != nil {
		return err
	}

	d.sanitize()

	err = d.autoTag()
	if err != nil {
		return err
	}

	r, err := sqlDocument.updateDraft.Tx(tx).Exec(
		data.Arg("id", d.ID),
		data.Arg("language", d.Language),
		data.Arg("version", d.Version),
		data.Arg("updater", d.editor.ID),
		data.Arg("title", d.Title),
		data.Arg("content", d.Content),
	)

	if err != nil {
		return err
	}

	rows, err := r.RowsAffected()
	if err != nil {
		return err
	}

	if rows == 0 {
		return errDocumentConflict
	}

	_, err = sqlDocument.deleteDraftTags.Tx(tx).Exec(
		data.Arg("draft_id", d.ID),
		data.Arg("language", d.Language),
	)
	if err != nil {
		return err
	}

	for i := range d.Tags {
		_, err = sqlDocument.insertDraftTag.Tx(tx).Exec(
			data.Arg("draft_id", d.ID),
			data.Arg("language", d.Tags[i].Language),
			data.Arg("tag", d.Tags[i].Value),
			data.Arg("stem", d.Tags[i].Stem),
			data.Arg("type", d.Tags[i].Type),
		)
		if err != nil {
			return err
		}

	}
	d.Version++

	return nil
}

// make history turns the current document version into a history record
func (d *Document) makeHistory() *DocumentHistory {
	return &DocumentHistory{
		DocumentContent: DocumentContent{
			ID:       d.ID,
			Version:  d.Version,
			Language: d.Language,
			Title:    d.Title,
			Content:  d.Content,

			// Creator of the history record is updater of the document
			Created: d.Updated,
			creator: d.updater,
		},
	}
}

// link builds weighted links to other published documents based on their tags
func (d *Document) link(tx *sql.Tx) error {
	//TODO:
	return nil
}

// index adds the document to the search index
func (d *Document) index(tx *sql.Tx) error {
	//TODO:
	return nil
}

func (d *DocumentDraft) canPublish() error {
	if d.editor == nil {
		return errDocumentUpdateAccess
	}

	if d.editor.IsAdmin() {
		return nil
	}

	if d.editor.ID == d.creator && d.DocumentContent.ID.IsNil() {
		// Brand new document
		return nil
	}

	count := 0
	err := sqlDocument.canPublish.QueryRow(
		data.Arg("document_id", d.DocumentContent.ID),
		data.Arg("user_id", d.editor.ID),
		data.Arg("creator", d.editor.ID),
	).Scan(&count)

	if err != nil {
		return err
	}

	if count == 0 {
		return errDocumentUpdateAccess
	}

	return nil
}

// Publish publishes a draft turing a draft into a document
func (d *DocumentDraft) Publish() (*Document, error) {
	err := d.canPublish()
	if err != nil {
		return nil, err
	}

	err = d.validate()
	if err != nil {
		return nil, err
	}

	var new *Document
	err = data.BeginTx(func(tx *sql.Tx) error {
		if d.DocumentContent.ID.IsNil() {
			// new document
			new = d.makeDocument(nil)
			err := new.insert(tx)

			if err != nil {
				return err
			}
		} else {
			old, err := documentGet(d.DocumentContent.ID, d.DocumentContent.Language)
			if err == errDocumentNotFound {
				// new language version of existing document
				err = d.DocumentContent.insert(tx)
				if err != nil {
					return err
				}
			} else {
				if err != nil {
					return err
				}

				err = old.makeHistory().insert(tx)
				if err != nil {
					return err
				}
				new = d.makeDocument(old)
				err = new.update(tx, d.editor)

				if err != nil {
					return err
				}
			}
		}

		err := d.delete(tx)
		if err != nil {
			return err
		}

		err = new.link(tx)
		if err != nil {
			return err
		}

		return new.index(tx)
	})

	if err != nil {
		return nil, err
	}

	return new, nil
}

// makeDocument creates a document record from the current document draft
func (d *DocumentDraft) makeDocument(currentDocument *Document) *Document {
	if currentDocument == nil {
		return &Document{
			accessor: d.editor,
			DocumentContent: DocumentContent{
				ID:       data.NewID(),
				Language: d.DocumentContent.Language,
				Version:  0,
				Title:    d.DocumentContent.Title,
				Content:  d.DocumentContent.Content,
				Tags:     d.DocumentContent.Tags,
				Updated:  time.Now(),
				Created:  time.Now(),
				creator:  d.editor.ID,
				updater:  d.editor.ID,
			},
			Created: time.Now(),
			creator: d.editor.ID,
		}
	}

	newDoc := *currentDocument
	newDoc.DocumentContent = d.DocumentContent

	// use the current documents version instead of the draft's version
	// NOTE: this is confusing, and I might consider tracking draft version separate from document version
	// rather than letting them overlap
	newDoc.Version = currentDocument.Version
	newDoc.Updated = time.Now()
	newDoc.updater = d.editor.ID
	newDoc.accessor = d.editor

	return &newDoc
}

// Delete deletes a draft
func (d *DocumentDraft) Delete() error {
	if d.editor == nil || d.editor.ID != d.creator {
		return errDocumentUpdateAccess
	}

	return data.BeginTx(func(tx *sql.Tx) error {
		return d.delete(tx)
	})
}

func (d *DocumentDraft) delete(tx *sql.Tx) error {
	if tx == nil {
		panic("A transaction is required when deleting a draft")
	}
	_, err := sqlDocument.deleteDraftTags.Tx(tx).Exec(
		data.Arg("draft_id", d.ID),
		data.Arg("language", d.Language),
	)
	if err != nil {
		return err
	}
	_, err = sqlDocument.deleteDraft.Tx(tx).Exec(data.Arg("id", d.ID), data.Arg("language", d.Language))

	return err
}

func (h *DocumentHistory) insert(tx *sql.Tx) error {
	_, err := sqlDocument.insertHistory.Tx(tx).Exec(
		data.Arg("document_id", h.ID),
		data.Arg("language", h.Language),
		data.Arg("version", h.Version),
		data.Arg("title", h.Title),
		data.Arg("content", h.Content),
		data.Arg("created", h.Created),
		data.Arg("creator", h.creator),
	)

	return err
}

func (d *Document) insert(tx *sql.Tx) error {
	if tx == nil {
		panic("A transaction is required when inserting a document")
	}

	_, err := sqlDocument.insert.Tx(tx).Exec(
		data.Arg("id", d.ID),
		data.Arg("created", d.Created),
		data.Arg("creator", d.creator),
	)
	if err != nil {
		return err
	}

	return d.DocumentContent.insert(tx)
}

func (d *DocumentContent) insert(tx *sql.Tx) error {
	if tx == nil {
		panic("A transaction is required when inserting a document")
	}
	_, err := sqlDocument.insertContent.Tx(tx).Exec(
		data.Arg("document_id", d.ID),
		data.Arg("language", d.Language),
		data.Arg("version", d.Version),
		data.Arg("title", d.Title),
		data.Arg("content", d.Content),
		data.Arg("created", d.Created),
		data.Arg("creator", d.creator),
		data.Arg("updated", d.Updated),
		data.Arg("updater", d.updater),
	)
	if err != nil {
		return err
	}

	for i := range d.Tags {
		_, err = sqlDocument.insertTag.Tx(tx).Exec(
			data.Arg("document_id", d.ID),
			data.Arg("language", d.Tags[i].Language),
			data.Arg("tag", d.Tags[i].Value),
			data.Arg("stem", d.Tags[i].Stem),
			data.Arg("type", d.Tags[i].Type),
		)
		if err != nil {
			return err
		}
	}
	return nil
}

func (d *DocumentContent) update(tx *sql.Tx, who *User) error {
	if tx == nil {
		panic("A transaction is required when updating a document")
	}

	if who == nil {
		return errDocumentUpdateAccess
	}

	r, err := sqlDocument.update.Tx(tx).Exec(
		data.Arg("title", d.Title),
		data.Arg("content", d.Content),
		data.Arg("updater", who.ID),
		data.Arg("document_id", d.ID),
		data.Arg("version", d.Version),
		data.Arg("language", d.Language),
	)

	if err != nil {
		return err
	}

	rows, err := r.RowsAffected()
	if err != nil {
		return err
	}

	if rows == 0 {
		return errDocumentConflict
	}

	_, err = sqlDocument.deleteTags.Tx(tx).Exec(
		data.Arg("document_id", d.ID),
		data.Arg("language", d.Language),
	)
	if err != nil {
		return err
	}

	for i := range d.Tags {
		_, err = sqlDocument.insertTag.Tx(tx).Exec(
			data.Arg("document_id", d.ID),
			data.Arg("language", d.Tags[i].Language),
			data.Arg("tag", d.Tags[i].Value),
			data.Arg("stem", d.Tags[i].Stem),
			data.Arg("type", d.Tags[i].Type),
		)
		if err != nil {
			return err
		}

	}

	return nil
}

// AddGroup adds a new group to a document, or updates an existing document group
func (d *Document) AddGroup(groupID data.ID, canPublish bool) error {
	if d.accessor == nil || d.accessor.ID != d.creator {
		return errDocumentUpdateAccess
	}

	if groupID.IsNil() {
		return NewFailure("Group ID is empty")
	}
	count := 0
	err := sqlDocument.groupExists.QueryRow(
		data.Arg("document_id", d.ID),
		data.Arg("group_id", groupID),
	).Scan(&count)
	if err != nil {
		return err
	}

	if count != 0 {
		_, err := sqlDocument.updateGroup.Exec(
			data.Arg("document_id", d.ID),
			data.Arg("group_id", groupID),
			data.Arg("can_publish", canPublish),
		)

		return err
	}

	result, err := sqlDocument.insertGroup.Exec(
		data.Arg("document_id", d.ID),
		data.Arg("group_id", groupID),
		data.Arg("can_publish", canPublish),
	)
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rows == 0 {
		return ErrGroupNotFound
	}

	return nil
}

// RemoveGroup removes a group from the document
func (d *Document) RemoveGroup(groupID data.ID) error {
	if d.accessor == nil || d.accessor.ID != d.creator {
		return errDocumentUpdateAccess
	}

	if groupID.IsNil() {
		return NewFailure("Group ID is empty")
	}

	_, err := sqlDocument.deleteGroup.Exec(data.Arg("document_id", d.ID), data.Arg("group_id", groupID))

	return err
}
