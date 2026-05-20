# Learn the basics of Ghost

> Run `ghost tutorial` to step through this tutorial live in the CLI.

This guided tour walks through the core Ghost workflow: create a database, load data, fork it, change the fork, compare the results, and clean up. Each step shows the exact `ghost` command the live tutorial runs and the output you can expect to see.

Throughout this guide, the temporary databases are named `tutorial-example` and `tutorial-example-fork`. The live `ghost tutorial` command generates a random suffix instead.

## Step 1 — Create a database

```bash
ghost create --name tutorial-example --wait
```

```
Created database 'tutorial-example'
ID: abc1234567
Connection: postgresql://tsdbadmin:<password>@<host>:5432/tsdb?sslmode=require
```

## Step 2 — Add sample data with SQL

The sql command connects to the database and executes the query you provide.

```bash
ghost sql tutorial-example \
  "CREATE TABLE ghost_tutorial_items (id serial PRIMARY KEY, name text NOT NULL, location text NOT NULL);
   INSERT INTO ghost_tutorial_items (name, location) VALUES ('apples', 'original'), ('bananas', 'original'), ('carrots', 'original');"
```

```
CREATE TABLE
INSERT 0 3
```

## Step 3 — Query the original database

```bash
ghost sql tutorial-example "SELECT id, name, location FROM ghost_tutorial_items ORDER BY id;"
```

```
 id │ name    │ location 
────┼─────────┼──────────
 1  │ apples  │ original 
 2  │ bananas │ original 
 3  │ carrots │ original 
(3 rows)
```

## Step 4 — Fork the database

Forking creates an independent copy you can safely experiment with.

```bash
ghost fork tutorial-example --name tutorial-example-fork --wait
```

```
Forked 'tutorial-example' → 'tutorial-example-fork'
ID: def1234567
Connection: postgresql://tsdbadmin:<password>@<host>:5432/tsdb?sslmode=require
```

## Step 5 — Mutate the fork

These changes are made only on the fork.

```bash
ghost sql tutorial-example-fork \
  "INSERT INTO ghost_tutorial_items (name, location) VALUES ('dragonfruit', 'fork');
   UPDATE ghost_tutorial_items SET location = 'fork' WHERE name = 'bananas';"
```

```
INSERT 0 1
UPDATE 1
```

## Step 6 — Compare the original and the fork

First, query the original database:

```bash
ghost sql tutorial-example "SELECT id, name, location FROM ghost_tutorial_items ORDER BY id;"
```

```
 id │ name    │ location 
────┼─────────┼──────────
 1  │ apples  │ original 
 2  │ bananas │ original 
 3  │ carrots │ original 
(3 rows)
```

Now query the fork. Notice the extra row and updated value:

```bash
ghost sql tutorial-example-fork "SELECT id, name, location FROM ghost_tutorial_items ORDER BY id;"
```

```
 id │ name        │ location 
────┼─────────────┼──────────
 1  │ apples      │ original 
 2  │ bananas     │ fork     
 3  │ carrots     │ original 
 4  │ dragonfruit │ fork     
(4 rows)
```

## Step 7 — Delete the tutorial databases

When the main steps finish, the live tutorial asks whether to delete the databases. To run the cleanup step yourself, use the following.

```bash
ghost delete tutorial-example-fork --confirm
ghost delete tutorial-example --confirm
```

```
Deleted 'tutorial-example-fork' (def1234567)
Deleted 'tutorial-example' (abc1234567)
```
