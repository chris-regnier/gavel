import sqlite3

def get_user(db_path, username):
    conn = sqlite3.connect(db_path)
    cursor = conn.cursor()
    query = f"SELECT * FROM users WHERE username = '{username}'"
    cursor.execute(query)
    results = cursor.fetchall()
    conn.close()
    return results

def login(db_path, username, password):
    users = get_user(db_path, username)
    for user in users:
        if user[2] == password:
            return True
    return False
