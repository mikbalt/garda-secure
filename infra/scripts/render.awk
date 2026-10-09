# Render a config template: replaces __NAME__ with the value of the
# environment variable NAME (literal replacement, values never pass through
# a command line). Unknown placeholders are left untouched.
#   awk -f render.awk template > output
# For values placed inside double-quoted YAML strings use __YAML_NAME__:
# backslashes and double quotes are escaped.
function yaml_escape(s,    r, i, c) {
  # Character loop instead of gsub: backslash handling in gsub replacements
  # differs between busybox awk and gawk.
  r = ""
  for (i = 1; i <= length(s); i++) {
    c = substr(s, i, 1)
    if (c == "\\" || c == "\"") r = r "\\"
    r = r c
  }
  return r
}
{
  line = $0
  out = ""
  while (match(line, /__[A-Z][A-Z0-9_]*__/)) {
    name = substr(line, RSTART + 2, RLENGTH - 4)
    token = substr(line, RSTART, RLENGTH)
    if (name ~ /^YAML_/) {
      var = substr(name, 6)
      value = (var in ENVIRON) ? yaml_escape(ENVIRON[var]) : token
    } else {
      value = (name in ENVIRON) ? ENVIRON[name] : token
    }
    out = out substr(line, 1, RSTART - 1) value
    line = substr(line, RSTART + RLENGTH)
  }
  print out line
}
