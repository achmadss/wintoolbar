# Works out the next version and the release notes for a draft release.
#
#   .\tools\release.ps1                      # prints and writes version.txt, notes.md
#   .\tools\release.ps1 -VersionOut v.txt -NotesOut n.md
#
# Versioning is Conventional Commits over the commits since the last v* tag:
#   BREAKING CHANGE footer or a `!` before the colon -> major
#   feat                                              -> minor
#   anything else (fix, perf, docs, chore, ...)       -> patch
# The first release, with no tag yet, is 0.3.0.
param(
    [string]$VersionOut = 'version.txt',
    [string]$NotesOut = 'notes.md'
)
$ErrorActionPreference = 'Stop'

$tags = @(git tag --list 'v*' --sort=-v:refname)
$lastTag = if ($tags.Count -gt 0) { $tags[0] } else { $null }
$range = if ($lastTag) { "$lastTag..HEAD" } else { 'HEAD' }

$sepField, $sepRecord = [char]0x1f, [char]0x1e
$raw = (git log $range --pretty=format:"%H$sepField%s$sepField%b$sepRecord") -join "`n"
$commits = @()
foreach ($record in ($raw -split [string]$sepRecord)) {
    $record = $record.Trim("`r", "`n")
    if (-not $record) { continue }
    $parts = $record -split [string]$sepField, 3
    $commits += [pscustomobject]@{
        Hash    = $parts[0]
        Subject = $parts[1]
        Body    = if ($parts.Count -gt 2) { $parts[2] } else { '' }
    }
}
if ($commits.Count -eq 0) {
    throw "No commits in $range, nothing to release."
}

# Next version from the last tag, or the first release when there is none.
if ($lastTag) {
    if ($lastTag -notmatch '^v?(\d+)\.(\d+)\.(\d+)$') {
        throw "Last tag '$lastTag' is not vMAJOR.MINOR.PATCH."
    }
    [int]$major, [int]$minor, [int]$patch = $Matches[1], $Matches[2], $Matches[3]
} else {
    $major, $minor, $patch = 0, 3, 0
}

$breaking, $added, $fixed, $changed, $other = @(), @(), @(), @(), @()
foreach ($c in $commits) {
    $isBreaking = $c.Body -match '(?m)^BREAKING[ -]CHANGE:'
    if ($c.Subject -match '^([a-zA-Z]+)(\([^)]*\))?(!)?:\s*(.+)$') {
        $type, $text = $Matches[1].ToLower(), $Matches[4]
        if ($Matches[3]) { $isBreaking = $true }
        if ($isBreaking) {
            $breaking += $c.Subject # one line per breaking change, not repeated below
        } else {
            switch ($type) {
                'feat' { $added += $text }
                'fix' { $fixed += $text }
                { $_ -in 'perf', 'refactor', 'style', 'build', 'ci', 'docs', 'chore', 'test' } { $changed += $text }
                default { $other += $text }
            }
        }
    } else {
        $other += $c.Subject
    }
}

if ($lastTag) {
    if ($breaking.Count -gt 0) {
        $major++
        $minor = 0
        $patch = 0
    } elseif ($added.Count -gt 0) {
        $minor++
        $patch = 0
    } else {
        $patch++
    }
}
$version = "$major.$minor.$patch"

$notes = [System.Collections.Generic.List[string]]::new()
$notes.Add('## Changes')
$sections = [ordered]@{
    'Breaking changes' = $breaking
    'Added'            = $added
    'Fixed'            = $fixed
    'Changed'          = $changed
    'Other'            = $other
}
foreach ($name in $sections.Keys) {
    $items = @($sections[$name] | Select-Object -Unique)
    if ($items.Count -eq 0) { continue }
    $notes.Add('')
    $notes.Add("### $name")
    foreach ($item in $items) { $notes.Add("- $item") }
}
$text = ($notes -join "`n") + "`n"

$version | Set-Content $VersionOut -NoNewline
$text | Set-Content $NotesOut -Encoding utf8NoBOM
Write-Output "version: $version (from $range)"
Write-Output $text
