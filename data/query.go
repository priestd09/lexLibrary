// Copyright (c) 2017-2018 Townsourced Inc.

package data

import (
	"bytes"
	"database/sql"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/mattn/go-sqlite3"
	"github.com/pkg/errors"
)

var queryBuildQueue []*Query

// Query is a templated query that can run across
// multiple database backends
type Query struct {
	statement string
	args      []string
	tx        *sql.Tx
	hasIn     bool
}

// Argument is a wrapper around sql.NamedArg so that a data behavior can be unified across all database backends
// mainly dateTime handling.  Always use the data.Arg function, and not the type directly
type Argument sql.NamedArg

// Arg defines an argument for use in a Lex Library query, and makes sure that data behaviors are consistent across
// multiple database backends
func Arg(name string, value interface{}) Argument {
	switch v := value.(type) {
	case time.Time:
		value = v.UTC()
	case NullTime:
		if v.Valid {
			v.Time = v.Time.UTC()
			value = v
		}
	}

	return Argument(sql.Named(name, value))
}

// Args creates an array of args under the same name, useful for in queries
func Args(name string, value interface{}) []Argument {
	val := reflect.ValueOf(value)
	kind := val.Kind()

	if kind != reflect.Slice && kind != reflect.Array {
		panic("func Args can only be used with slices and arrays")
	}

	args := make([]Argument, val.Len())

	for i := range args {
		args[i] = Argument(sql.Named(inArgName(name, i), val.Index(i).Interface()))
	}

	return args
}

// NewQuery creates a new query from the template passed in
func NewQuery(tmpl string) *Query {
	q := &Query{
		statement: tmpl,
	}

	if db != nil {
		q.buildTemplate()
	} else {
		queryBuildQueue = append(queryBuildQueue, q)
	}

	return q
}

func (q Query) orderedArgs(args []Argument) []interface{} {
	ordered := make([]interface{}, 0, len(q.args))

	for i := range q.args {
		for j := range args {
			if args[j].Name == q.args[i] {
				switch dbType {
				case postgres, cockroachdb, sqlserver:
					// named args
					ordered = append(ordered, sql.NamedArg(args[j]))
				default:
					// unnamed values
					ordered = append(ordered, args[j].Value)
				}
				break
			}
		}
	}
	return ordered
}

func (q Query) argPlaceholder(name string) string {
	switch dbType {
	case postgres, cockroachdb:
		return "$" + strconv.Itoa(len(q.args))
	case sqlserver:
		return "@" + name
	default:
		return "?"
	}
}

func (q *Query) buildTemplate() {
	if db == nil {
		panic("Can't build query templates before the database type is set")
	}
	funcs := template.FuncMap{
		"arg": func(name string) string {
			// Args must be named and must be unique, and must use sql.Named
			if name == "" {
				panic("Arguments must be named in sql statements")
			}

			for i := range q.args {
				if name == q.args[i] {
					panic(fmt.Sprintf("%s already exists in the query arguments", name))
				}
			}

			q.args = append(q.args, name)
			if strings.HasPrefix(name, "...") {
				q.hasIn = true
				// arguments are an in statement set at runtime
				return fmt.Sprintf(`{{inArgs "%s"}}`, strings.TrimPrefix(name, "..."))
			}
			return q.argPlaceholder(name)
		},
		"bytes":    bytesColumn,
		"datetime": datetimeColumn,
		"text":     textColumn,
		"varchar":  varcharColumn,
		"id":       idColumn,
		"int":      intColumn,
		"bool":     boolColumn,
		"defaultDateTime": func() string {
			t := time.Time{}
			switch dbType {
			case mysql, mariadb:
				return t.Format("2006-01-02 15:04:05.000")
			case sqlite:
				return t.Format(sqlite3.SQLiteTimestampFormats[0])
			case postgres, cockroachdb:
				return t.Format(time.RFC3339)
			case sqlserver:
				return t.Format(time.RFC3339)
			default:
				panic("Unsupported database type")
			}
		},
		"NOW": func() string {
			t := time.Now().UTC()
			switch dbType {
			case mysql, mariadb:
				return fmt.Sprintf("'%s'", t.Format("2006-01-02 15:04:05.000"))
			case sqlite:
				return fmt.Sprintf("'%s'", t.Format(sqlite3.SQLiteTimestampFormats[0]))
			case postgres, cockroachdb:
				return fmt.Sprintf("'%s'", t.Format(time.RFC3339))
			case sqlserver:
				return fmt.Sprintf("'%s'", t.Format(time.RFC3339))
			default:
				panic("Unsupported database type")
			}
		},
		"TRUE": func() string {
			switch dbType {
			case mysql, mariadb, postgres, cockroachdb:
				return "true"
			case sqlite, sqlserver:
				return "1"
			default:
				panic("Unsupported database type")
			}
		},
		"FALSE": func() string {
			switch dbType {
			case mysql, mariadb, postgres, cockroachdb:
				return "false"
			case sqlite, sqlserver:
				return "0"
			default:
				panic("Unsupported database type")
			}
		},
		"db": DatabaseType,
		"sqlite": func() bool {
			return dbType == sqlite
		},
		"postgres": func() bool {
			return dbType == postgres
		},
		"mysql": func() bool {
			return dbType == mysql
		},
		"mariadb": func() bool {
			return dbType == mariadb
		},
		"cockroachdb": func() bool {
			return dbType == cockroachdb
		},
		"sqlserver": func() bool {
			return dbType == sqlserver
		},
		"limit": func() string {
			switch dbType {
			case sqlite, postgres, cockroachdb:
				return `"limit"`
			case mysql, mariadb:
				return "`limit`"
			case sqlserver:
				return "[limit]"
			default:
				panic("Unsupported database type")
			}
		},
	}

	q.parseStatement(funcs)
}

func (q *Query) parseStatement(funcs template.FuncMap) {
	buff := bytes.NewBuffer([]byte{})
	tmpl, err := template.New("").Funcs(funcs).Parse(q.statement)
	if err != nil {
		panic(fmt.Errorf("Error parsing query template '%s': %s", q.statement, err))
	}
	err = tmpl.Execute(buff, nil)
	if err != nil {
		panic(fmt.Errorf("Error building query template'%s': %s", q.statement, err))
	}

	q.statement = strings.TrimSpace(buff.String())
}

// Exec executes a templated query without returning any rows
func (q Query) Exec(args ...Argument) (result sql.Result, err error) {
	if q.statement == "" {
		q.buildTemplate()
	}
	q = q.expandIn(args...)

	if q.tx != nil {
		result, err = q.tx.Exec(q.statement, q.orderedArgs(args)...)
	} else {
		result, err = db.Exec(q.statement, q.orderedArgs(args)...)
	}

	if err != nil {
		return nil, errors.Wrapf(err, "Executing query: \n%s\n", q.Statement())
	}
	return result, nil
}

// Query executes a templated query that returns rows
func (q Query) Query(args ...Argument) (rows *sql.Rows, err error) {
	if q.statement == "" {
		panic("Query template hasn't been built yet")
	}

	q = q.expandIn(args...)
	if q.tx != nil {
		rows, err = q.tx.Query(q.statement, q.orderedArgs(args)...)
	} else {
		rows, err = db.Query(q.statement, q.orderedArgs(args)...)
	}

	if err != nil {
		return nil, errors.Wrapf(err, "Executing query: \n%s\n", q.Statement())
	}
	return rows, nil
}

// Row wraps sql.Row so that custom errors can be passed through with query.QueryRow
type Row struct {
	row       *sql.Row
	err       error
	statement string
}

func (r *Row) Scan(dest ...interface{}) error {
	if r.err != nil {
		return r.err
	}
	err := r.row.Scan(dest...)
	if err == sql.ErrNoRows {
		return err
	}
	if err != nil {
		return errors.Wrapf(err, "Executing query: \n%s\n", r.statement)
	}
	return nil
}

// QueryRow executes a templated query that returns a single row
func (q Query) QueryRow(args ...Argument) *Row {
	if q.statement == "" {
		panic("Query template hasn't been built yet")
	}

	q = q.expandIn(args...)
	r := &Row{
		statement: q.Statement(),
	}

	if q.tx != nil {
		r.row = q.tx.QueryRow(q.statement, q.orderedArgs(args)...)
	} else {
		r.row = db.QueryRow(q.statement, q.orderedArgs(args)...)
	}

	return r
}

// Tx returns a new copy of the query that runs in the passed in transaction if a transaction is passed in
// if tx is nil then the normal query is returned
func (q Query) Tx(tx *sql.Tx) Query {
	if tx == nil {
		return q
	}
	q.tx = tx
	return q
}

// Statement returns the complied query template
func (q Query) Statement() string {
	if q.statement == "" {
		panic("Query template hasn't been built yet")
	}
	return q.statement
}

func (q Query) String() string {
	return q.Statement()
}

func inArgName(name string, i int) string {
	return name + ":" + strconv.Itoa(i)
}

func (q Query) expandIn(args ...Argument) Query {
	if !q.hasIn {
		return q
	}

	q.parseStatement(template.FuncMap{
		"inArgs": func(name string) string {
			in := ""
			for a := range q.args {
				if q.args[a] == "..."+name {
					var inArgs []string
					for i := range args {
						inName := inArgName(name, len(inArgs))
						if args[i].Name == inName {
							if i != 0 {
								in += ", "
							}
							in += q.argPlaceholder(inName)
							inArgs = append(inArgs, inName)
						}
					}
					// expand the single argument by replacing it with a slice of numbered
					// arguments in the runtime argument slice
					// copy to prevent side effects
					wArgs := make([]string, len(q.args))
					q.args = append(wArgs[:a], append(inArgs, wArgs[a+1:]...)...)
					break
				}
			}

			return in
		},
	})
	return q
}

// BeginTx begins a transaction on the database
// If the function passed in returns an error, the transaction rolls back
// If it returns a nil error, then the transaction commits
func BeginTx(trnFunc func(tx *sql.Tx) error) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}

	err = trnFunc(tx)
	if err != nil {
		rErr := tx.Rollback()
		if rErr != nil {
			return errors.Errorf("Error rolling back transaction.  Rollback error %s, Original error %s",
				rErr, err)
		}
		return err
	}

	err = tx.Commit()
	if err != nil {
		return errors.Wrap(err, "Error committing transaction")
	}

	return nil
}

// PrintRows pretty prints the result set from passed in rows
func PrintRows(rows *sql.Rows, padding int) (string, error) {
	result := ""

	columns, err := rows.Columns()
	if err != nil {
		return "", err
	}

	lengths := make([]int, len(columns))

	wrap := ""
	cols := ""
	for i := range columns {
		lengths[i] = padding + len(columns[i])
		cols += fmt.Sprintf("%-"+strconv.Itoa(lengths[i])+"s", columns[i])
		for j := 0; j < lengths[i]; j++ {
			wrap += "-"
		}
	}

	result += wrap + "\n" + cols + "\n" + wrap + "\n"

	values := make([]interface{}, len(columns))

	scanArgs := make([]interface{}, len(values))
	for i := range values {
		scanArgs[i] = &values[i]
	}
	count := 0
	defer rows.Close()
	for rows.Next() {
		err = rows.Scan(scanArgs...)
		if err != nil {
			panic(err)
		}

		for i := range columns {
			var str string
			switch values[i].(type) {
			case nil:
				str = "NULL"
			case []byte:
				str = string(values[i].([]byte))
			default:
				str = fmt.Sprintf("%v", values[i])
			}

			val := fmt.Sprintf("%-"+strconv.Itoa(lengths[i])+"s", str)
			// don't trim last column
			if i != len(columns)-1 && len(str) > padding {
				val = val[:lengths[i]-3] + "..."
			}
			result += val

		}
		result += "\n"
		count++
	}

	return result + wrap + "\n(" + strconv.Itoa(count) + " rows)\n", nil
}

// Debug runs the passed in query and returns a string of the results
// in a tab delimited format, with columns listed in the first row
// meant for debugging use. Will panic instead of throwing an error
func (q Query) Debug(args ...Argument) string {
	rows, err := q.Query(args...)
	if err != nil {
		panic(err)
	}

	result, err := PrintRows(rows, 25)
	if err != nil {
		panic(err)
	}
	return result
}

// DebugPrint prints out the debug query to the screen
func (q Query) DebugPrint(args ...Argument) {
	fmt.Println(q.Debug(args...))
}

func prepareQueries() error {
	for i := range queryBuildQueue {
		queryBuildQueue[i].buildTemplate()
	}
	return nil
}
