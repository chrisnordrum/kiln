# Kiln reference

Full-stack web language. No client state, no effects, no lifecycle.
A page is a pure function of (params, session, data). An interaction is a
named server action -> mutation -> re-query -> patch.

## §syntax

2-space indent, no tabs. Blocks are indentation. `#` starts a comment.
Tables are `PascalCase`; routes, actions, fields are `snake_case`.
A view nests at most 6 levels below its page; files cap at 120 lines.

## §expr

Literals: `"text"` `123` `1.5` `true` `false` `null`
Refs: `params.id` `session.user` `Task[expr].field` `t.field` (loop var)
      `items.price` reads one column of a list, for `sum` and `join`.
Ops: `== != < <= > >= and or not + - * / %` (`+` joins text, and a path
     joins with anything to build a URL)
Conditional: `if cond then a else b`
Calls: `fn(a, b)` — stdlib only. No loops, no recursion, no user functions.
Every expression provably terminates.

## §stdlib

text: `upper(s) lower(s) title(s) trim(s) slug(s) truncate(s,n) len(s)`
      `join(col,sep) has(s,sub) replace(s,a,b)`
num:  `abs(n) round(n) floor(n) ceil(n) min(a,b) max(a,b) sum(col)`
      `count(list) money(n) pct(n)`
date: `now() ago(at) date(at,"Y-m-d") days_between(a,b) plus_days(at,n)`
misc: `coalesce(a,b) plural(n,"item","items") default(v,d)`

## §app

`app.kiln` — one per program.

    app tasks
      title   "Tasks"
      session user ref User
      effect  email via smtp
      effect  webhook via https

`session <name> ref <Table>` declares what `session.<name>` resolves to.
`effect` declares the only I/O a program may perform; actions reach it with
`send <effect> to=e ...`. There is no other way out of the language.

## §schema

`schema/*.kiln`

    table Task
      id       id
      project  ref Project on delete cascade
      title    text max 200
      notes    text?
      status   enum todo doing done = "todo"
      rank     int
      done     bool = false
      created  at now
      index project, rank
      unique project, title

Types: `id` `text [max N]` `int` `num` `bool` `at` `enum a b c`
       `ref Table [on delete cascade|restrict|null]`
Modifiers: `?` nullable, `= v` default, `at now` defaults to insert time.

## §actions

`actions/*.kiln`

    action toggle_task
      in
        id    ref Task
        done  bool
      allow session.user == Task[id].project.owner
      do
        set Task[id].done = done
      after
        refresh

`in` declares typed params; callers must supply every one. `?` marks one
nullable, which is how an empty form field arrives — without it a blank input
stores "" and every reader has to handle both empties.
`allow` must be true or the call is denied (enforced at the boundary).
`do` statements: `set T[e].f = e` `set session.x = e` `new T f=e ...` `del T[e]`
              `send effect to=e ...`
`after`: `refresh` (re-run the route's data) `goto /path` `toast "msg"`

## §routes

`routes/*.kiln` — one screen, one file.

    route project_detail
      path /projects/:id
      guard session.user else redirect /login
      data
        project  one   Project where id == params.id
        tasks    many  Task where project == params.id order rank asc
      view
        page title=project.title
          head 2 "Tasks"
          each tasks as t
            row gap=2
              check value=t.done do=toggle_task id=t.id done=$value
              text t.title
          form do=add_task project=params.id
            input title text required max=200
            submit "Add task"

`data`: `key one|many Table where <expr> [order f asc|desc] [limit n] [offset e]`
`one` yields a record (404 if absent); `many` yields a list.
`$value` is the event value inside an interactive element.

## §view

Layout:  `page title=e` `col` `row` `grid cols=n` `card` `sep`
Content: `head 1..4 "text"` `text e` `rich e` `img src=e alt=e` `badge e` `empty "msg"`
Control: `each <list> as <v>` `when <expr>` + `else`
Action:  `link "label" to=/path` `button "label" do=action arg=e [confirm="msg"]`
         `to=` takes an expression. Link to a record by building the path:
         `to="/projects/" + p.id`. A field name inside quotes is text, not a
         value, so `to="/projects/p.id"` is the same dead link on every row.
         `check value=e do=action ...` `form do=action arg=e`
         `input name type [required] [max=n] [label="..."]` `select name from=list`
         `area name [rows=n]` `submit "label"`

Attributes: `gap=0..6` `pad=0..6` `align=start|center|end` `style=<token>`
These four are reserved on every element, so `do=` elements never pass them on.
Style tokens: `plain quiet strong danger good warn`
No CSS, no class names. The token set is closed.

## §tests

`tests/*.kiln`

    test "toggle marks done"
      seed Task id=1 done=false
      as user=1
      call toggle_task id=1 done=true
      expect Task[1].done == true
      expect denied when as user=2

Route tests: `visit /path` then `expect text "..."`, `expect no text "..."`
(it must not render), or `expect redirect /login` (the guard refused).

## §cli

    kiln check          verify whole program (--json for machine output)
    kiln fmt            rewrite to canonical form
    kiln map            whole-app outline: routes, tables, actions, relations
    kiln where <sym>    every definition and use of a symbol
    kiln snap           render every route a test visits, as stable text;
                        --check diffs the committed snapshots
    kiln test           run tests/
    kiln dev            dev server; --data <file> keeps the store
                        across restarts
    kiln new route|table|action <name>
    kiln docs [--section <name>]
    kiln explain K021   what an error code means and how to fix it

## §checked

`kiln check` proves, before anything runs:
no reference to a field that does not exist; every `do=` names a real action
with every `in` param supplied at the right type; every `allow` rule is
satisfiable from its route's guard; every form field matches the action's `in`;
every internal link resolves to a real route path; every style token is real.
