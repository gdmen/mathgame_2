# generate_models.py

import argparse
import os
import re

CAMEL_TO_SNAKE_RE = re.compile(r'(?<!^)(?=[A-Z][a-z])')

def camel_to_snake(s: str) -> str:
    return CAMEL_TO_SNAKE_RE.sub('_', s).lower()

def get_model_string(m: dict) -> str:
    import_time = False
    key_name = ""
    key_type = ""
    unique_struct_fields = []
    struct_fields = []
    non_key_struct_fields = []
    auto_incr_struct_fields = []
    create_struct_fields = []
    sql_fields = []
    non_key_sql_fields = []
    auto_incr_sql_fields = []
    create_sql_fields = []
    unique_sql_fields = []
    has_user_fk = False
    for f in m["fields"]:
        if "PRIMARY KEY" in f["sql"]:
            key_name = f["name"]
            key_type = f["type"]
        if f["name"] == "UserId":
            has_user_fk = True
    has_additional_user_fk =  has_user_fk and key_name != "UserId"
    for f in m["fields"]:
        if any(x in f["sql"] for x in ["TIMESTAMP", "DATE", "DATETIME"]):
            import_time = True
        n = f["name"]
        snake_n = camel_to_snake(n)
        struct_fields.append(n)
        sql_fields.append(snake_n)
        if n != key_name:
            non_key_struct_fields.append(n)
            non_key_sql_fields.append(snake_n)
        if "AUTO_INCREMENT" in f["sql"]:
            auto_incr_struct_fields.append(n)
            auto_incr_sql_fields.append(snake_n)
        elif not "DEFAULT" in f["sql"]:
            create_struct_fields.append(n)
            create_sql_fields.append(snake_n)
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
'''
    if import_time:
        s += '''    "time"
'''
    s += ''')
'''

    # const start
    s += "const ("

    # create table SQL
    s += '''
    Create{0}TableSQL = `
    CREATE TABLE {1} (
        {2}
    ) DEFAULT CHARSET=utf8mb4 ;`
'''.format(
        m["name"].capitalize(),
        m["table"],
        ",\n\t".join(["%s %s" % (camel_to_snake(f["name"]), f["sql"]) for f in m["fields"]])
    )

    # create SQL
    s += '''
    create{0}SQL = `INSERT INTO {1} ({2}) VALUES ({3});`
'''.format(
        m["name"].capitalize(),
        m["table"],
        ", ".join(create_sql_fields),
        ", ".join(["?"]*len(create_sql_fields))
    )

    # get SQL
    s += '''
    get{0}SQL = `SELECT * FROM {1} WHERE {2}=?{3};`
'''.format(
        m["name"].capitalize(),
        m["table"],
        camel_to_snake(key_name),
        " AND user_id=?" if has_additional_user_fk else ""
    )

    # get key SQL
    s += '''
    get{0}KeySQL = `SELECT {1} FROM {2}s WHERE {3};`
'''.format(
        m["name"].capitalize(),
        ", ".join(auto_incr_sql_fields),
        m["name"],
        " AND ".join([f+"=?" for f in create_sql_fields]),
    )

    # list SQL
    s += '''
    list{0}SQL = `SELECT * FROM {1}{2};`
'''.format(
        m["name"].capitalize(),
        m["table"],
        " WHERE user_id=?" if has_user_fk else ""
    )

    # update SQL
    s += '''
    update{0}SQL = `UPDATE {1} SET {2} WHERE {3}=?{4};`
'''.format(
        m["name"].capitalize(),
        m["table"],
        ", ".join([f+"=?" for f in non_key_sql_fields]),
        camel_to_snake(key_name),
        " AND user_id=?" if has_additional_user_fk else ""
    )

    # delete SQL
    s += '''
    delete{0}SQL = `DELETE FROM {1} WHERE {2}=?{3};`
'''.format(
        m["name"].capitalize(),
        m["table"],
        camel_to_snake(key_name),
        " AND user_id=?" if has_additional_user_fk else ""
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
    s += '''
    func (m *{0}Manager) Create(model *{0}) (int, string, error) {{
        status := http.StatusCreated
        {1}, err := m.DB.Exec(create{0}SQL, {2})
        if err != nil {{
            if !strings.Contains(err.Error(), "Duplicate entry") {{
                msg := "Couldn't add {3} to database"
                return http.StatusInternalServerError, msg, err
            }}
'''.format(
        m["name"].capitalize(),
        "result" if key_name in auto_incr_struct_fields else "_",
        ", ".join(["model." + n for n in create_struct_fields]),
        m["name"]
    )

    if auto_incr_struct_fields:
        s += '''
            // Update model with the configured return field.
            err = m.DB.QueryRow(get{0}KeySQL, {1}).Scan({2})
            if err != nil {{
                msg := "Couldn't add {3} to database"
                return http.StatusInternalServerError, msg, err
            }}
'''.format(
        m["name"].capitalize(),
        ", ".join(["model." + n for n in create_struct_fields]),
        ", ".join(["&model." + n for n in auto_incr_struct_fields]),
        m["name"]
    )

    s += '''
            return http.StatusOK, "", nil
        }
    '''

    if key_name in auto_incr_struct_fields:
        s += '''
        last_id, err := result.LastInsertId()
        if err != nil {{
            msg := "Couldn't add {0} to database"
            return http.StatusInternalServerError, msg, err
        }}
        model.{1} = {2}(last_id)
'''.format(
        m["name"],
        key_name,
        key_type
    )

    s += '''
        return status, "", nil
    }
'''

    # manager.Get()
    s += '''
    func (m *{0}Manager) Get({1} {2}{5}) (*{0}, int, string, error) {{
        model := &{0}{{}}
        err := m.DB.QueryRow(get{0}SQL, {1}{6}).Scan({3})
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
        m["name"],
        ", user_id uint32" if has_additional_user_fk else "",
        ", user_id" if has_additional_user_fk else ""
    )

    # manager.List() and manager.CustomList(sql)
    list_code = '''
        defer rows.Close()
        if err != nil {{
            msg := "Couldn't get {1} from database"
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
        m["table"],
        ", ".join(["&model." + n for n in struct_fields])
    )

    # manager.CustomIdList(sql)
    list_id_code = '''
        defer rows.Close()
        if err != nil {{
            msg := "Couldn't get {1} from database"
            return nil, http.StatusInternalServerError, msg, err
        }}
        for rows.Next() {{
            var {2} {3}
            err = rows.Scan(&{2})
            if err != nil {{
                msg := "Couldn't scan row from database"
                return nil, http.StatusInternalServerError, msg, err
            }}
            ids = append(ids, {2})
        }}
        err = rows.Err()
        if err != nil {{
            msg := "Error scanning rows from database"
            return nil, http.StatusInternalServerError, msg, err
        }}
        return &ids, http.StatusOK, "", nil
    }}
'''.format(
        m["name"].capitalize(),
        m["table"],
        camel_to_snake(key_name),
        key_type
    )

    # manager.List()
    s += '''
    func (m *{0}Manager) List({1}) (*[]{0}, int, string, error) {{
        models := []{0}{{}}
        rows, err := m.DB.Query(list{0}SQL{2})
'''.format(
        m["name"].capitalize(),
        "user_id uint32" if has_user_fk else "",
        ", user_id" if has_user_fk else ""
    )
    s += list_code

    # manager.CustomList()
    s += '''
    func (m *{0}Manager) CustomList(sql string) (*[]{0}, int, string, error) {{
        models := []{0}{{}}
        rows, err := m.DB.Query(sql)
'''.format(
        m["name"].capitalize())
    s += list_code

    # manager.CustomIdList()
    s += '''
    func (m *{0}Manager) CustomIdList(sql string) (*[]{2}, int, string, error) {{
        ids := []{2}{{}}
        sql = "SELECT {1} FROM problems WHERE " + sql
        rows, err := m.DB.Query(sql)
'''.format(
        m["name"].capitalize(),
        camel_to_snake(key_name),
        key_type
    )
    s += list_id_code

    # manager.CustomSql()
    s += '''
    func (m *{0}Manager) CustomSql(sql string) (int, string, error) {{
        _, err := m.DB.Query(sql)
        if err != nil {{
			msg := "Couldn't run sql for {0} in database"
			return http.StatusBadRequest, msg, err
        }}
        return http.StatusOK, "", nil
    }}
'''.format(
        m["name"].capitalize())

    # manager.Update()
    s += '''
    func (m *{0}Manager) Update(model *{0}{4}) (int, string, error) {{
        // Check for 404s
        _, status, msg, err := m.Get(model.{1}{5})
        if err != nil {{
            return status, msg, err
        }}
        // Update
        _, err = m.DB.Exec(update{0}SQL, {2}{5})
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
        m["name"],
        ", user_id uint32" if has_additional_user_fk else "",
        ", user_id" if has_additional_user_fk else ""
    )

    # manager.Delete()
    s += '''
    func (m *{0}Manager) Delete({1} {2}{4}) (int, string, error) {{
        result, err := m.DB.Exec(delete{0}SQL, {1}{5})
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
        m["name"],
        ", user_id uint32" if has_additional_user_fk else "",
        ", user_id" if has_additional_user_fk else ""
    )

    return s

def main():
    parser = argparse.ArgumentParser(description="Generate a go model for the math game.")
    parser.add_argument("-c", "--config", metavar="config", type=str, help="name of the config file (models.json)", required=True)
    parser.add_argument("-o", "--output", metavar="output", type=str, help="name of the output directory", required=True)
    args = parser.parse_args()
    c = {}
    with open(args.config, "r") as f:
        import json
        c = json.loads(f.read())
    for m in c["models"]:
        with open(os.path.join(args.output,  m["name"]+"_model.generated.go"), "w") as f:
            f.write(get_model_string(m))

if __name__ == "__main__":
    main()
