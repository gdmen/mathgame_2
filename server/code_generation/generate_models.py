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

    # Named template vars for readability
    model_name = m["name"].capitalize()
    table = m["table"]
    table_def = ",\n\t".join(["%s %s" % (camel_to_snake(f["name"]), f["sql"]) for f in m["fields"]])
    create_fields = ", ".join(create_sql_fields)
    create_placeholders = ", ".join(["?"] * len(create_sql_fields))
    key_snake = camel_to_snake(key_name)
    user_id_cond = " AND user_id=?" if has_additional_user_fk else ""
    get_key_cols = ", ".join(auto_incr_sql_fields)
    get_key_table = m["name"]
    get_key_where = " AND ".join([f + "=?" for f in create_sql_fields])
    list_user_cond = " WHERE user_id=?" if has_user_fk else ""
    update_set = ", ".join([f + "=?" for f in non_key_sql_fields])

    # const start
    s += "const ("

    # create table SQL
    s += f'''
    Create{model_name}TableSQL = `
    CREATE TABLE {table} (
        {table_def}
    ) DEFAULT CHARSET=utf8mb4 ;`
'''

    # create SQL
    s += f'''
    create{model_name}SQL = `INSERT INTO {table} ({create_fields}) VALUES ({create_placeholders});`
'''

    # get SQL
    s += f'''
    get{model_name}SQL = `SELECT * FROM {table} WHERE {key_snake}=?{user_id_cond};`
'''

    # get key SQL
    s += f'''
    get{model_name}KeySQL = `SELECT {get_key_cols} FROM {get_key_table}s WHERE {get_key_where};`
'''

    # list SQL
    s += f'''
    list{model_name}SQL = `SELECT * FROM {table}{list_user_cond};`
'''

    # update SQL
    s += f'''
    update{model_name}SQL = `UPDATE {table} SET {update_set} WHERE {key_snake}=?{user_id_cond};`
'''

    # delete SQL
    s += f'''
    delete{model_name}SQL = `DELETE FROM {table} WHERE {key_snake}=?{user_id_cond};`
'''

    # const end
    s += ")\n"

    # model struct
    s += f"type {model_name} struct {{"
    for f in m["fields"]:
        field_name = f["name"]
        field_type = f["type"]
        field_json = camel_to_snake(f["name"])
        form_tag = f' form:"{field_json}"' if f["name"] != key_name else ""
        s += f'\n{field_name} {field_type} `json:"{field_json}" uri:"{field_json}"{form_tag}`'
    s += "\n}"

    # model.String()
    fmt_args = ", ".join([n + ": %v" for n in struct_fields])
    fmt_vals = ", ".join(["model." + n for n in struct_fields])
    s += f'''
    func (model {model_name}) String() string {{
        return fmt.Sprintf("{fmt_args}", {fmt_vals})
    }}
'''

    # manager struct
    s += f'''
    type {model_name}Manager struct {{
        DB *sql.DB
    }}
'''

    # manager.Create()
    create_exec_result = "result" if key_name in auto_incr_struct_fields else "_"
    create_args = ", ".join(["model." + n for n in create_struct_fields])
    entity_name = m["name"]
    s += f'''
    func (m *{model_name}Manager) Create(model *{model_name}) (int, string, error) {{
        status := http.StatusCreated
        {create_exec_result}, err := m.DB.Exec(create{model_name}SQL, {create_args})
        if err != nil {{
            if !strings.Contains(err.Error(), "Duplicate entry") {{
                msg := "Couldn't add {entity_name} to database"
                return http.StatusInternalServerError, msg, err
            }}
'''

    if auto_incr_struct_fields:
        get_key_scan_args = ", ".join(["&model." + n for n in auto_incr_struct_fields])
        s += f'''
            // Update model with the configured return field.
            err = m.DB.QueryRow(get{model_name}KeySQL, {create_args}).Scan({get_key_scan_args})
            if err != nil {{
                msg := "Couldn't add {entity_name} to database"
                return http.StatusInternalServerError, msg, err
            }}
'''

    s += '''
            return http.StatusOK, "", nil
        }
    '''

    if key_name in auto_incr_struct_fields:
        s += f'''
        last_id, err := result.LastInsertId()
        if err != nil {{
            msg := "Couldn't add {entity_name} to database"
            return http.StatusInternalServerError, msg, err
        }}
        model.{key_name} = {key_type}(last_id)
'''

    s += '''
        return status, "", nil
    }
'''

    # manager.Get()
    get_scan_args = ", ".join(["&model." + n for n in struct_fields])
    get_user_param = ", user_id uint32" if has_additional_user_fk else ""
    get_user_arg = ", user_id" if has_additional_user_fk else ""
    s += f'''
    func (m *{model_name}Manager) Get({key_snake} {key_type}{get_user_param}) (*{model_name}, int, string, error) {{
        model := &{model_name}{{}}
        err := m.DB.QueryRow(get{model_name}SQL, {key_snake}{get_user_arg}).Scan({get_scan_args})
        if err == sql.ErrNoRows {{
            msg := "Couldn't find a {entity_name} with that {key_snake}"
            return nil, http.StatusNotFound, msg, err
        }} else if err != nil {{
            msg := "Couldn't get {entity_name} from database"
            return nil, http.StatusInternalServerError, msg, err
        }}
	    return model, http.StatusOK, "", nil
    }}
'''

    # manager.List() and manager.CustomList(sql)
    list_scan_args = ", ".join(["&model." + n for n in struct_fields])
    list_code = f'''
        defer rows.Close()
        if err != nil {{
            msg := "Couldn't get {table} from database"
            return nil, http.StatusInternalServerError, msg, err
        }}
        for rows.Next() {{
            model := {model_name}{{}}
            err = rows.Scan({list_scan_args})
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
'''

    # manager.CustomIdList(sql)
    id_var = key_snake
    list_id_code = f'''
        defer rows.Close()
        if err != nil {{
            msg := "Couldn't get {table} from database"
            return nil, http.StatusInternalServerError, msg, err
        }}
        for rows.Next() {{
            var {id_var} {key_type}
            err = rows.Scan(&{id_var})
            if err != nil {{
                msg := "Couldn't scan row from database"
                return nil, http.StatusInternalServerError, msg, err
            }}
            ids = append(ids, {id_var})
        }}
        err = rows.Err()
        if err != nil {{
            msg := "Error scanning rows from database"
            return nil, http.StatusInternalServerError, msg, err
        }}
        return &ids, http.StatusOK, "", nil
    }}
'''

    # manager.List()
    list_param = "user_id uint32" if has_user_fk else ""
    list_arg = ", user_id" if has_user_fk else ""
    s += f'''
    func (m *{model_name}Manager) List({list_param}) (*[]{model_name}, int, string, error) {{
        models := []{model_name}{{}}
        rows, err := m.DB.Query(list{model_name}SQL{list_arg})
'''
    s += list_code

    # manager.CustomList()
    s += f'''
    func (m *{model_name}Manager) CustomList(sql string) (*[]{model_name}, int, string, error) {{
        models := []{model_name}{{}}
        sql = "SELECT * FROM {table} WHERE " + sql
        rows, err := m.DB.Query(sql)
'''
    s += list_code

    # manager.CustomIdList()
    s += f'''
    func (m *{model_name}Manager) CustomIdList(sql string) (*[]{key_type}, int, string, error) {{
        ids := []{key_type}{{}}
        sql = "SELECT {key_snake} FROM {table} WHERE " + sql
        rows, err := m.DB.Query(sql)
'''
    s += list_id_code

    # manager.CustomSql()
    s += f'''
    func (m *{model_name}Manager) CustomSql(sql string) (int, string, error) {{
        _, err := m.DB.Query(sql)
        if err != nil {{
			msg := "Couldn't run sql for {model_name} in database"
			return http.StatusBadRequest, msg, err
        }}
        return http.StatusOK, "", nil
    }}
'''

    # manager.Update()
    update_args = ", ".join(["model." + n for n in non_key_struct_fields + [key_name]])
    update_user_param = ", user_id uint32" if has_additional_user_fk else ""
    update_user_arg = ", user_id" if has_additional_user_fk else ""
    s += f'''
    func (m *{model_name}Manager) Update(model *{model_name}{update_user_param}) (int, string, error) {{
        // Check for 404s
        _, status, msg, err := m.Get(model.{key_name}{update_user_arg})
        if err != nil {{
            return status, msg, err
        }}
        // Update
        _, err = m.DB.Exec(update{model_name}SQL, {update_args}{update_user_arg})
        if err != nil {{
			msg := "Couldn't update {entity_name} in database"
			return http.StatusInternalServerError, msg, err
        }}
        return http.StatusOK, "", nil
    }}
'''

    # manager.Delete()
    delete_user_param = ", user_id uint32" if has_additional_user_fk else ""
    delete_user_arg = ", user_id" if has_additional_user_fk else ""
    s += f'''
    func (m *{model_name}Manager) Delete({key_snake} {key_type}{delete_user_param}) (int, string, error) {{
        result, err := m.DB.Exec(delete{model_name}SQL, {key_snake}{delete_user_arg})
        if err != nil {{
            msg := "Couldn't delete {entity_name} in database"
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
'''

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
