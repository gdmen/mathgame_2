# generate_model.py

import argparse
import re

CAMEL_TO_SNAKE_RE = re.compile(r'(?<!^)(?=[A-Z][a-z])')

def camel_to_snake(s: str) -> str:
    return CAMEL_TO_SNAKE_RE.sub('_', s).lower()

def get_model_string(m: dict) -> str:

    key_name = ""
    key_type = ""
    key_auto = False
    unique_struct_fields = []
    struct_fields = []
    non_key_struct_fields = []
    sql_fields = []
    non_key_sql_fields = []
    unique_sql_fields = []
    for f in m["fields"]:
        if "PRIMARY KEY" in f["sql"]:
            key_name = f["name"]
    for f in m["fields"]:
        n = f["name"]
        snake_n = camel_to_snake(n)
        struct_fields.append(n)
        sql_fields.append(snake_n)
        if n == key_name:
            key_type = f["type"]
            key_auto = "AUTO_INCREMENT" in f["sql"]
        else:
            non_key_struct_fields.append(n)
            non_key_sql_fields.append(snake_n)
        if "UNIQUE" in f["sql"]:
            unique_struct_fields.append(n)
            unique_sql_fields.append(snake_n)

    # file comment & imports
    s = '''// Package api contains api routes, handlers, and models
package api // import "garydmenezes.com/mathgame/server/api"

import (
    "database/sql"
    "fmt"
    "net/http"
    "strings"
)
'''

    # const start
    s += "const ("

    # create table SQL
    s += '''
    Create{0}TableSQL = `
    CREATE TABLE {1}s (
        {2}
    ) DEFAULT CHARSET=utf8 ;`
'''.format(
        m["name"].capitalize(),
        m["name"],
        ",\n\t".join(["%s %s" % (camel_to_snake(f["name"]), f["sql"]) for f in m["fields"]])
    )

    # create SQL
    create_sql_fields = non_key_sql_fields if key_auto else sql_fields
    s += '''
    create{0}SQL = `INSERT INTO {1}s ({2}) VALUES ({3});`
'''.format(
        m["name"].capitalize(),
        m["name"],
        ", ".join(create_sql_fields),
        ", ".join(["?"]*len(create_sql_fields))
    )

    # get SQL
    s += '''
    get{0}SQL = `SELECT * FROM {1}s WHERE {2}=?;`
'''.format(
        m["name"].capitalize(),
        m["name"],
        camel_to_snake(key_name)
    )

    # get key SQL
    s += '''
    get{0}KeySQL = `SELECT {1} FROM {2}s WHERE {3};`
'''.format(
        m["name"].capitalize(),
        camel_to_snake(key_name),
        m["name"],
        ", ".join([f+"=?" for f in unique_sql_fields]),
    )

    # list SQL
    s += '''
    list{0}SQL = `SELECT * FROM {1}s;`
'''.format(
        m["name"].capitalize(),
        m["name"]
    )

    # update SQL
    s += '''
    update{0}SQL = `UPDATE {1}s SET {2} WHERE {3}=?;`
'''.format(
        m["name"].capitalize(),
        m["name"],
        ", ".join([f+"=?" for f in non_key_sql_fields]),
        camel_to_snake(key_name)
    )

    # delete SQL
    s += '''
    delete{0}SQL = `DELETE FROM {1}s WHERE {2}=?;`
'''.format(
        m["name"].capitalize(),
        m["name"],
        camel_to_snake(key_name)
    )

    # const end
    s += ")\n"

    # model struct
    s += "type {0} struct {{".format(m["name"].capitalize())
    for f in m["fields"]:
        fstr = '\n{0} {1} `json:"{2}" uri:"{2}"'
        if f["name"] != key_name:
            fstr += ' form:"{2}"'
        fstr += "`"
        s += fstr.format(f["name"], f["type"], camel_to_snake(f["name"]))
    s += "\n}"

    # model.String()
    s += '''
    func (model {0}) String() string {{
        return fmt.Sprintf("{1}", {2})
    }}
'''.format(
        m["name"].capitalize(),
        ", ".join([n + ": %v" for n in struct_fields]),
        ", ".join(["model." + n for n in struct_fields])
    )

    # manager struct
    s += '''
    type {0}Manager struct {{
        DB *sql.DB
    }}
'''.format(
        m["name"].capitalize()
    )

    # manager.Create()
    create_struct_fields = non_key_struct_fields if key_auto else struct_fields
    s += '''
    func (m *{0}Manager) Create(model *{0}) (int, string, error) {{
        status := http.StatusCreated
        _, err := m.DB.Exec(create{0}SQL, {1})
        if err != nil {{
            if !strings.Contains(err.Error(), "Duplicate entry") {{
                msg := "Couldn't add {2} to database"
                return http.StatusInternalServerError, msg, err
            }}
            status = http.StatusOK
        }}
        // Update model with the key of the already existing model
        _ = m.DB.QueryRow(get{0}KeySQL, {3}).Scan(&model.{4})
        return status, "", nil
    }}
'''.format(
        m["name"].capitalize(),
        ", ".join(["model." + n for n in create_struct_fields]),
        m["name"],
        ", ".join(["model." + n for n in unique_struct_fields]),
        key_name
    )

    # manager.Get()
    s += '''
    func (m *{0}Manager) Get({1} {2}) (*{0}, int, string, error) {{
        model := &{0}{{}}
        err := m.DB.QueryRow(get{0}SQL, {1}).Scan({3})
        if err == sql.ErrNoRows {{
            msg := "Couldn't find a {4} with that {1}"
            return nil, http.StatusNotFound, msg, err
        }} else if err != nil {{
            msg := "Couldn't get {4} from database"
            return nil, http.StatusInternalServerError, msg, err
        }}
	    return model, http.StatusOK, "", nil
    }}
'''.format(
        m["name"].capitalize(),
        camel_to_snake(key_name),
        key_type,
        ", ".join(["&model." + n for n in struct_fields]),
        m["name"]
    )

    # manager.List()
    s += '''
    func (m *{0}Manager) List() (*[]{0}, int, string, error) {{
        models := []{0}{{}}
        rows, err := m.DB.Query(list{0}SQL)
        defer rows.Close()
        if err != nil {{
            msg := "Couldn't get {1}s from database"
            return nil, http.StatusInternalServerError, msg, err
        }}
        for rows.Next() {{
            model := {0}{{}}
            err = rows.Scan({2})
            if err != nil {{
                msg := "Couldn't scan row from database"
                return nil, http.StatusInternalServerError, msg, err
            }}
            models = append(models, model)
        }}
        err = rows.Err()
        if err != nil {{
            msg := "Error scanning rows from database"
            return nil, http.StatusInternalServerError, msg, err
        }}
        return &models, http.StatusOK, "", nil
    }}
'''.format(
        m["name"].capitalize(),
        m["name"],
        ", ".join(["&model." + n for n in struct_fields])
    )

    # manager.Update()
    s += '''
    func (m *{0}Manager) Update(model *{0}) (int, string, error) {{
        // Check for 404s
        _, status, msg, err := m.Get(model.{1})
        if err != nil {{
            return status, msg, err
        }}
        // Update
        _, err = m.DB.Exec(update{0}SQL, {2})
        if err != nil {{
			msg := "Couldn't update {3} in database"
			return http.StatusInternalServerError, msg, err
        }}
        return http.StatusOK, "", nil
    }}
'''.format(
        m["name"].capitalize(),
        key_name,
        ", ".join(["model." + n for n in non_key_struct_fields + [key_name]]),
        m["name"]
    )

    # manager.Delete()
    s += '''
    func (m *{0}Manager) Delete({1} {2}) (int, string, error) {{
        result, err := m.DB.Exec(delete{0}SQL, {1})
        if err != nil {{
            msg := "Couldn't delete {3} in database"
            return http.StatusInternalServerError, msg, err
        }}
        // Check for 404s
        // Ignore errors (if the database doesn't support RowsAffected)
        affected, _ := result.RowsAffected()
        if affected == 0 {{
            return http.StatusNotFound, "", nil
        }}
        return http.StatusNoContent, "", nil
    }}
'''.format(
        m["name"].capitalize(),
        camel_to_snake(key_name),
        key_type,
        m["name"]
    )

    return s

def main():
    parser = argparse.ArgumentParser(description="Generate a golang model for the math game.")
    parser.add_argument("-c", "--config", metavar="config", type=str, help="name of the config file (models.json)", required=True)
    args = parser.parse_args()
    c = {}
    with open(args.config, "r") as f:
        import json
        c = json.loads(f.read())
    for m in c["models"]:
        with open(m["name"]+"_model.go", "w") as f:
            f.write(get_model_string(m))

if __name__ == "__main__":
    main()
