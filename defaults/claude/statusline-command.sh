#!/bin/sh
input=$(cat)

# --- Model ---
model=$(echo "$input" | jq -r '.model.id // "unknown"')

# --- Context usage ---
used_pct=$(echo "$input" | jq -r '.context_window.used_percentage // empty')
if [ -n "$used_pct" ]; then
  ctx_str=$(printf "%.0f%% ctx" "$used_pct")
else
  ctx_str="? ctx"
fi

# --- Token count (compact notation) ---
total_in=$(echo "$input" | jq -r '.context_window.total_input_tokens // 0')
total_out=$(echo "$input" | jq -r '.context_window.total_output_tokens // 0')
total_tokens=$((total_in + total_out))
if [ "$total_tokens" -ge 1000 ]; then
  tok_str=$(awk "BEGIN { printf \"%.1fk tokens\", $total_tokens/1000 }")
else
  tok_str="${total_tokens} tokens"
fi

# --- Timestamp ---
ts=$(date +%H:%M:%S)

os=$(uname -s)

if [ "$os" = "Darwin" ]; then
  # --- Battery (macOS pmset) ---
  batt_raw=$(pmset -g batt 2>/dev/null | grep -o '[0-9]*%' | head -1)
  if [ -n "$batt_raw" ]; then
    batt_str="🔋 ${batt_raw}"
  else
    batt_str="🔋 ?"
  fi

  # --- CPU (macOS top) ---
  cpu_raw=$(top -l 1 -n 0 2>/dev/null | awk '/CPU usage/ { gsub(/%[^,]*/,"",$0); user=$3; sys=$5; gsub(/%/,"",user); gsub(/%/,"",sys); printf "%.0f", user+sys; exit }')
  if [ -n "$cpu_raw" ]; then
    cpu_str="CPU ${cpu_raw}%"
  else
    cpu_str="CPU ?%"
  fi

  # --- RAM (macOS vm_stat) ---
  mem_str="RAM ?"
  vm=$(vm_stat 2>/dev/null)
  if [ -n "$vm" ]; then
    page_size=$(echo "$vm" | awk '/page size/ { print $8 }')
    active=$(echo "$vm"    | awk '/Pages active:/    { gsub(/\./,"",$3); print $3 }')
    wired=$(echo "$vm"     | awk '/Pages wired down:/ { gsub(/\./,"",$4); print $4 }')
    compressed=$(echo "$vm"| awk '/Pages occupied by compressor:/ { gsub(/\./,"",$5); print $5 }')
    if [ -n "$page_size" ] && [ -n "$active" ] && [ -n "$wired" ]; then
      compressed=${compressed:-0}
      mem_str=$(awk "BEGIN { used=($active+$wired+$compressed)*$page_size/1073741824; printf \"RAM %.1fGB\", used }")
    fi
  fi

else
  # Linux (Docker container)

  # No battery in a container
  batt_str="🔌"

  # --- CPU (/proc/stat, two samples 0.2s apart for instantaneous %) ---
  cpu_str="CPU ?%"
  if [ -f /proc/stat ]; then
    s1=$(awk '/^cpu / {print $2,$3,$4,$5,$6,$7,$8}' /proc/stat)
    sleep 0.2
    s2=$(awk '/^cpu / {print $2,$3,$4,$5,$6,$7,$8}' /proc/stat)
    cpu_raw=$(awk -v s1="$s1" -v s2="$s2" 'BEGIN {
      n=split(s1,a); split(s2,b)
      t1=0; t2=0
      for(i=1;i<=n;i++) { t1+=a[i]; t2+=b[i] }
      dt=t2-t1
      if(dt>0) printf "%.0f", 100*(dt-(b[4]-a[4]))/dt
    }')
    [ -n "$cpu_raw" ] && cpu_str="CPU ${cpu_raw}%"
  fi

  # --- RAM (/proc/meminfo) ---
  mem_str="RAM ?"
  if [ -f /proc/meminfo ]; then
    mem_total=$(awk '/^MemTotal:/{print $2}' /proc/meminfo)
    mem_avail=$(awk '/^MemAvailable:/{print $2}' /proc/meminfo)
    if [ -n "$mem_total" ] && [ -n "$mem_avail" ]; then
      mem_str=$(awk "BEGIN { printf \"RAM %.1fGB\", ($mem_total-$mem_avail)*1024/1073741824 }")
    fi
  fi
fi

printf "%s | %s | %s | %s | %s | %s | %s" \
  "$model" "$ctx_str" "$tok_str" "$ts" "$batt_str" "$cpu_str" "$mem_str"
