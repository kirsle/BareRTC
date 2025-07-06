#!/usr/bin/env python

"""
SQLite to Postgres migration script.

BareRTC originally used SQLite directly for the DirectMessageHistory database,
but has switched to use GORM so that you can use Postgres instead of SQLite.

This script will migrate an existing BareRTC database.sqlite script over to
Postgres.

Instructions:

1. Create a Python virtualenv and install dependencies

    mkvirtualenv barertc
    pip install -r scripts/requirements.txt

2. Run this script

    python3 scripts/sqlite2psql.py --sqlite database.sqlite \
        --psql 'user=barertc password=barertc dbname=barertc sslmode=disable TimeZone=America/Los_Angeles'

Note: this script does not create the Postgres table. Run BareRTC in Postgres
mode and have it create the table first, then run this script to backfill your
old DMs from SQLite3.
"""

import argparse
import datetime
import sqlite3
import psycopg2

def main(args):

    print("Opening SQLite DB")
    sqlite = open_sqlite3(args.sqlite)

    print("Connecting to Postgres")
    psql = open_postgres(args.psql)

    migrate(sqlite, psql)


def migrate(sqlite, psql):
    print("Migrating direct messages")

    counter = 0
    for row in sqlite_paginate(sqlite):
        counter += 1

        # Migrate legacy Timestamp field.
        if row['timestamp'] > 0:
            dt = datetime.datetime.fromtimestamp(row['timestamp'])
            row['timestamp'] = 0
            row['created_at'] = dt.isoformat()

        # No created_at? (shouldn't happen), use the message_id instead.
        if not row['created_at']:
            print("Missing created_at! Using message_id as timestamp!")
            dt = datetime.datetime.fromtimestamp(row['message_id'])
            row['created_at'] = dt.isoformat()

        # Upsert it into Postgres.
        psql_upsert(psql, row)
        if counter % 500 == 0:
            print(f"Migrated {counter} messages...")
            psql.commit()

    print("Finished!")
    psql.commit()


def sqlite_paginate(sqlite):
    """Paginate over the DMs from SQLite as a generator."""
    cur = sqlite.cursor()
    after_id = 0

    while True:
        res = cur.execute(f"""
            SELECT
                message_id,
                channel_id,
                username,
                message,
                timestamp
            FROM direct_messages
            WHERE message_id > {after_id}
            ORDER BY message_id ASC
            LIMIT 500
        """)
        page = res.fetchall()

        if len(page) == 0:
            return

        for row in page:
            after_id = row[0]
            yield dict(
                message_id=row[0],
                channel_id=row[1],
                username=row[2],
                message=row[3],
                timestamp=row[4],
                created_at=None,
            )


def psql_upsert(psql, row):
    cur = psql.cursor()
    cur.execute("""
        INSERT INTO direct_messages (message_id, channel_id, username, message, timestamp, created_at)
        VALUES (%s, %s, %s, %s, 0, %s)
        ON CONFLICT (message_id) DO NOTHING
    """, (
        row['message_id'], row['channel_id'], row['username'], row['message'], row['created_at'],
    ))
    psql.commit()

def open_sqlite3(connstr):
    conn = sqlite3.connect(connstr)
    return conn


def open_postgres(connstr):
    conn = psycopg2.connect(connstr)
    return conn

if __name__ == "__main__":
    parser = argparse.ArgumentParser("sqlite2psql")
    parser.add_argument("--sqlite",
        type=str,
        required=True,
        help="Path to your SQLite database",
    )
    parser.add_argument("--psql",
        type=str,
        required=True,
        help="Your Postgres connection string",
    )
    args = parser.parse_args()
    main(args)