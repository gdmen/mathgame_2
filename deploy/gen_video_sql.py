with open("videos.csv", "r") as csv:
    with open("videos.sql", "w") as sql:
            for l in csv:
                title, url = (s.strip() for s in l.split(","))
                sql.write('''INSERT INTO videos (title, url, start, end, enabled) VALUES ("%s", "%s", 0, 9999, 1);\n''' % (title, url))
