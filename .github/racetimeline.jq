# TEMPORARY (#7290 measurement): summarizes `go test -debug-trace` output.
# Pairs each action's begin and end, then prints busy seconds per kind of
# action, every test binary's run with its start and end, and the link of each
# long-running package.
def kind:
  if startswith("build check cache") then "cachecheck"
  elif startswith("test run ") then "run"
  elif startswith("link ") then "link"
  elif startswith("vet ") then "vet"
  elif startswith("build ") then "compile"
  else "other" end;
def short: sub("github.com/blairham/sh/?"; "") | if . == "" then "(root)" else . end;
def f1: (. * 10 | round) / 10;

map(select((.ph == "B" or .ph == "E") and (.name | startswith("Executing action ("))))
| (map(.ts) | min) as $t0
| group_by(.name + "|" + (.tid | tostring))
| map(sort_by(.ts) as $g | range(0; ($g | length) - 1; 2)
      | {a: ($g[.].name | sub("^Executing action \\("; "") | sub("\\)$"; "")),
         s: (($g[.].ts - $t0) / 1e6), e: (($g[. + 1].ts - $t0) / 1e6)})
| map(. + {k: (.a | kind), d: (.e - .s)})
| . as $all
| ($all | map(select(.k == "run")) | sort_by(.s)) as $runs
| ($all | map(select(.k == "link")) | map({key: (.a | sub("^link "; "") | sub("\\.test$"; "")), value: .}) | from_entries) as $links
| "-- busy seconds by action kind (sum of action durations)",
  ($all | group_by(.k) | map("\(.[0].k)\t\(map(.d) | add | f1)s\tn=\(length)") | .[]),
  "-- first run start \($runs[0].s | f1)s, last run end \($runs | map(.e) | max | f1)s",
  "-- test runs >= 3s, by start: start end dur package [link end]",
  ($runs | map(select(.d >= 3))[]
    | (.a | sub("^test run "; "")) as $p
    | "\(.s | f1)\t\(.e | f1)\t\(.d | f1)\t\($p | short)\t\($links[$p].e // null | if . then "link@\(f1)" else "" end)"),
  "-- mean actions in flight per 30s bucket: t runs compiles links vets",
  ([range(0; ($all | map(.e) | max) + 30; 30)] | .[] as $t
    | def busy($k): $all | map(select(.k == $k and .s < $t + 30 and .e > $t)
        | ([.e, $t + 30] | min) - ([.s, $t] | max)) | add // 0 | . / 30 | (. * 10 | round) / 10;
      "\($t)s\t\(busy("run"))\t\(busy("compile"))\t\(busy("link"))\t\(busy("vet"))")
