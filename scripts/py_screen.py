"""
Python reference implementation of the screening engine.
Direct port of pkg/screening/screening.go for benchmark comparison.

Usage:
    python3 scripts/py_screen.py data/eu_sample.json
    python3 scripts/py_screen.py data/eu_sample.json --name "Irina Kostenko"
"""

import json
import sys
import time
import unicodedata
from pathlib import Path


def normalize(name):
    name = name.lower().strip()
    return "".join(
        c
        for c in unicodedata.normalize("NFKD", name)
        if not unicodedata.combining(c)
    )


def jaro_winkler(s1, s2):
    if s1 == s2:
        return 1.0
    len1, len2 = len(s1), len(s2)
    if not len1 or not len2:
        return 0.0

    match_dist = max(len1, len2) // 2 - 1
    if match_dist < 0:
        match_dist = 0

    m1 = [False] * len1
    m2 = [False] * len2
    matches = 0
    for i in range(len1):
        start = max(0, i - match_dist)
        end = min(len2, i + match_dist + 1)
        for j in range(start, end):
            if m2[j]:
                continue
            if s1[i] == s2[j]:
                m1[i] = True
                m2[j] = True
                matches += 1
                break

    if matches == 0:
        return 0.0

    transpositions = 0
    k = 0
    for i in range(len1):
        if not m1[i]:
            continue
        while not m2[k]:
            k += 1
        if s1[i] != s2[k]:
            transpositions += 1
        k += 1

    jaro = (
        matches / len1
        + matches / len2
        + (matches - transpositions / 2) / matches
    ) / 3.0

    prefix_len = 0
    for i in range(min(len1, len2, 4)):
        if s1[i] == s2[i]:
            prefix_len += 1
        else:
            break

    return jaro + prefix_len * 0.1 * (1.0 - jaro)


def screen(name, persons, threshold=0.8):
    norm_name = normalize(name)
    matches = []
    for p in persons:
        best_score = 0.0
        best_type = "fuzzy"

        score = jaro_winkler(norm_name, normalize(p["name"]))
        if score > best_score:
            best_score = score

        for alias in p.get("aliases", []):
            score = jaro_winkler(norm_name, normalize(alias))
            if score > best_score:
                best_score = score

        if best_score >= 1.0:
            best_score = 1.0
            best_type = "exact"

        for alias in p.get("aliases", []):
            if normalize(alias) == norm_name:
                best_score = 0.95
                best_type = "alias"
                break

        if best_score >= threshold:
            matches.append(
                {
                    "person": p["name"],
                    "score": round(best_score, 2),
                    "type": best_type,
                }
            )

    matches.sort(key=lambda x: x["score"], reverse=True)
    return matches


def load_persons(path):
    data = json.loads(Path(path).read_text())
    if isinstance(data, dict):
        data = [data]
    return data


def main():
    if len(sys.argv) < 2:
        print(f"Usage: python3 {sys.argv[0]} <json_file> [--name NAME]")
        sys.exit(1)

    path = sys.argv[1]
    persons = load_persons(path)
    print(f"Loaded {len(persons)} entries from {path}")

    name = None
    if "--name" in sys.argv:
        idx = sys.argv.index("--name")
        name = sys.argv[idx + 1]

    if name:
        start = time.perf_counter()
        matches = screen(name, persons)
        elapsed = (time.perf_counter() - start) * 1000
        for m in matches:
            print(f"  [{m['score']:.2f}] {m['person']} ({m['type']})")
        print(f"\n{len(matches)} matches in {elapsed:.0f}ms")
    else:
        names = ["Irina Kostenko", "Vitaly Kulikov", "Vladimir Putin", "Sberbank"]
        for n in names:
            start = time.perf_counter()
            matches = screen(n, persons)
            elapsed = (time.perf_counter() - start) * 1000
            print(f"  {n}: {len(matches)} matches, {elapsed:.0f}ms")


if __name__ == "__main__":
    main()
