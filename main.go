package main

import (
	"database/sql"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/go-sql-driver/mysql"
)

type Table struct {
	Name    string            `json:"table_name"`
	Comment string            `json:"table_comment"`
	Fields  map[string]string `json:"fields"`
}

type Column struct {
	Name    string `json:"field"`
	Key     string `json:"key"`
	Comment string `json:"comment"`
}

type PageData struct {
	DSN       string
	DBName    string
	Tables    []*Table
	TableName string
	Columns   []*Column
	PK        string
}

func getTables(dsn string) (tables []*Table, err error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return
	}
	defer db.Close()

	cfg, err := mysql.ParseDSN(dsn)
	if err != nil {
		return
	}

	rows, err := db.Query("select table_name, table_comment from INFORMATION_SCHEMA.TABLES where table_schema = '" + cfg.DBName + "'")
	if err != nil {
		return
	}
	defer rows.Close()

	for rows.Next() {
		var table Table
		if err = rows.Scan(&table.Name, &table.Comment); err != nil {
			return
		}
		tables = append(tables, &table)
	}

	rows, err = db.Query("select table_name, column_name, column_comment from INFORMATION_SCHEMA.COLUMNS where table_schema = '" + cfg.DBName + "'")
	if err != nil {
		return
	}
	defer rows.Close()

	for rows.Next() {
		var tableName, columnName, columnComment string
		if err = rows.Scan(&tableName, &columnName, &columnComment); err != nil {
			return
		}
		for _, table := range tables {
			if table.Name == tableName {
				if table.Fields == nil {
					table.Fields = make(map[string]string)
				}
				table.Fields[columnName] = columnComment
				break
			}
		}
	}

	return
}

func getColumns(dsn string, tableName string) (columns []*Column, err error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return
	}
	defer db.Close()

	cfg, err := mysql.ParseDSN(dsn)
	if err != nil {
		return
	}

	rows, err := db.Query("select column_name, column_key, column_comment from INFORMATION_SCHEMA.COLUMNS where table_schema = '" + cfg.DBName + "' and table_name = '" + tableName + "'")
	if err != nil {
		return
	}
	defer rows.Close()

	for rows.Next() {
		var column Column
		if err = rows.Scan(&column.Name, &column.Key, &column.Comment); err != nil {
			return
		}
		columns = append(columns, &column)
	}
	return
}

func getPK(columns []*Column) (pk string, err error) {
	for _, column := range columns {
		if column.Key == "PRI" {
			pk = column.Name
			return
		}
	}
	err = fmt.Errorf("no primary key")
	return
}

func getData(dsn string, tableName string, search string, sort string, order string, offset string, limit string) (total int, data []map[string]any, err error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return
	}
	defer db.Close()

	query := "select count(*) from " + tableName
	if search != "" {
		query += " where " + search
	}

	row := db.QueryRow(query)
	err = row.Scan(&total)
	if err != nil {
		return
	}

	query = "select * from " + tableName
	if search != "" {
		query += " where " + search
	}
	if sort != "" {
		query += " order by " + sort
	}
	if sort != "" && order != "" {
		query += " " + order
	}
	if limit != "" {
		query += " limit " + limit
	}
	if offset != "" {
		query += " offset " + offset
	}

	rows, err := db.Query(query)
	if err != nil {
		return
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		return
	}

	count := len(columns)
	data = make([]map[string]any, 0)
	values := make([]any, count)
	valuePtrs := make([]any, count)
	for rows.Next() {
		for i := 0; i < count; i++ {
			valuePtrs[i] = &values[i]
		}
		rows.Scan(valuePtrs...)
		entry := make(map[string]any)
		for i, col := range columns {
			var v any
			val := values[i]
			b, ok := val.([]byte)
			if ok {
				v = string(b)
			} else {
				v = val
			}
			entry[col] = v
		}
		data = append(data, entry)
	}

	return
}

func add(dsn string, tableName string, postForm url.Values) (err error) {
	var fields, values []string
	for field := range postForm {
		fields = append(fields, field)
		values = append(values, postForm.Get(field))
	}

	query := "insert into " + tableName + " (" +
		strings.Join(fields, ", ") + ") values ('" + strings.Join(values, "', '") + "')"

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return
	}
	defer db.Close()

	_, err = db.Exec(query)

	return
}

func edit(dsn string, tableName string, postForm url.Values) (err error) {
	columns, err := getColumns(dsn, tableName)
	if err != nil {
		return
	}

	pk, err := getPK(columns)
	if err != nil {
		return
	}

	var pkValue string
	var fieldValues []string
	for field := range postForm {
		if field == pk {
			pkValue = postForm.Get(field)
		} else {
			fieldValues = append(fieldValues, field+" = '"+postForm.Get(field)+"'")
		}
	}

	query := "update " + tableName + " set " + strings.Join(fieldValues, ", ") + " where " + pk + " = " + pkValue

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return
	}
	defer db.Close()

	_, err = db.Exec(query)

	return
}

func clone(dsn string, tableName string, ids string) (err error) {
	columns, err := getColumns(dsn, tableName)
	if err != nil {
		return
	}

	pk, err := getPK(columns)
	if err != nil {
		return
	}

	var fields []string
	for _, column := range columns {
		if pk != column.Name {
			fields = append(fields, column.Name)
		}
	}

	query := "insert into " + tableName + " (" + strings.Join(fields, ", ") + ") select " + strings.Join(fields, ", ") + " from " + tableName + " where " + pk + " in (" + ids + ")"

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return
	}
	defer db.Close()

	_, err = db.Exec(query)

	return
}

func del(dsn string, tableName string, ids string) (err error) {
	columns, err := getColumns(dsn, tableName)
	if err != nil {
		return
	}

	pk, err := getPK(columns)
	if err != nil {
		return
	}

	query := "delete from " + tableName + " where " + pk + " in (" + ids + ")"

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return
	}
	defer db.Close()

	_, err = db.Exec(query)

	return
}

func main() {
	r := gin.Default()

	r.GET("/ping", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"message": "pong",
		})
	})

	r.Static("/assets", "./assets")

	r.LoadHTMLGlob("templates/*.tmpl")

	r.GET("/", func(c *gin.Context) {
		c.HTML(http.StatusOK, "index.tmpl", nil)
	})

	r.GET("/index", func(c *gin.Context) {
		c.HTML(http.StatusOK, "index.tmpl", nil)
	})

	r.GET("/index.htm", func(c *gin.Context) {
		c.HTML(http.StatusOK, "index.tmpl", nil)
	})

	r.GET("/index.html", func(c *gin.Context) {
		c.HTML(http.StatusOK, "index.tmpl", nil)
	})

	r.POST("/", func(c *gin.Context) {
		var data PageData

		dsn := c.PostForm("dsn")
		data.DSN = dsn

		if cfg, err := mysql.ParseDSN(dsn); err == nil {
			data.DBName = cfg.DBName
		}

		if tables, err := getTables(dsn); err == nil {
			data.Tables = tables
		}

		c.HTML(http.StatusOK, "index.tmpl", data)
	})

	r.GET("/structure", func(c *gin.Context) {
		var data PageData

		dsn := c.Query("dsn")
		data.DSN = dsn

		c.HTML(http.StatusOK, "structure.tmpl", data)
	})

	r.GET("/getStructure", func(c *gin.Context) {
		dsn := c.Query("dsn")

		tables, err := getTables(dsn)
		if err != nil {
			result := map[string]any{"code": 1, "msg": err.Error(), "data": nil}
			c.JSON(http.StatusOK, result)
			return
		}
		c.JSON(http.StatusOK, tables)
	})

	r.GET("/data", func(c *gin.Context) {
		var data PageData

		dsn := c.Query("dsn")
		data.DSN = dsn

		tableName := c.Query("tableName")
		data.TableName = tableName

		columns, err := getColumns(dsn, tableName)
		if err != nil {
			result := map[string]any{"code": 1, "msg": err.Error(), "data": nil}
			c.JSON(http.StatusOK, result)
			return
		}
		data.Columns = columns

		pk, err := getPK(columns)
		if err == nil {
			data.PK = pk
		}

		c.HTML(http.StatusOK, "data.tmpl", data)
	})

	r.GET("/getData", func(c *gin.Context) {
		dsn := c.Query("dsn")
		tableName := c.Query("tableName")
		search := c.Query("search")
		sort := c.Query("sort")
		order := c.Query("order")
		offset := c.Query("offset")
		limit := c.Query("limit")

		total, data, err := getData(dsn, tableName, search, sort, order, offset, limit)
		if err != nil {
			result := map[string]any{"code": 1, "msg": err.Error(), "data": nil}
			c.JSON(http.StatusOK, result)
			return
		}

		c.JSON(http.StatusOK, map[string]any{
			"total": total,
			"rows":  data,
		})
	})

	r.POST("/add", func(c *gin.Context) {
		dsn := c.Query("dsn")
		tableName := c.Query("tableName")
		c.MultipartForm()

		err := add(dsn, tableName, c.Request.PostForm)
		if err != nil {
			result := map[string]any{"code": 1, "msg": "添加失败", "data": err.Error()}
			c.JSON(http.StatusOK, result)
		} else {
			result := map[string]any{"code": 0, "msg": "添加成功", "data": nil}
			c.JSON(http.StatusOK, result)
		}
	})

	r.POST("/edit", func(c *gin.Context) {
		dsn := c.Query("dsn")
		tableName := c.Query("tableName")
		c.MultipartForm()

		err := edit(dsn, tableName, c.Request.PostForm)
		if err != nil {
			result := map[string]any{"code": 1, "msg": "编辑失败", "data": err.Error()}
			c.JSON(http.StatusOK, result)
		} else {
			result := map[string]any{"code": 0, "msg": "编辑成功", "data": nil}
			c.JSON(http.StatusOK, result)
		}
	})

	r.POST("/clone", func(c *gin.Context) {
		dsn := c.Query("dsn")
		tableName := c.Query("tableName")
		ids := c.PostForm("ids")

		err := clone(dsn, tableName, ids)
		if err != nil {
			result := map[string]any{"code": 1, "msg": "克隆失败", "data": err.Error()}
			c.JSON(http.StatusOK, result)
		} else {
			result := map[string]any{"code": 0, "msg": "克隆成功", "data": nil}
			c.JSON(http.StatusOK, result)
		}
	})

	r.POST("/del", func(c *gin.Context) {
		dsn := c.Query("dsn")
		tableName := c.Query("tableName")
		ids := c.PostForm("ids")

		err := del(dsn, tableName, ids)
		if err != nil {
			result := map[string]any{"code": 1, "msg": "删除失败", "data": err.Error()}
			c.JSON(http.StatusOK, result)
		} else {
			result := map[string]any{"code": 0, "msg": "删除成功", "data": nil}
			c.JSON(http.StatusOK, result)
		}
	})

	r.Run(":3306")
}
