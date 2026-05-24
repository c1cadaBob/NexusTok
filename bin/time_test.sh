#!/bin/bash
# time_test.sh - API 响应时间测试脚本
#
# 功能：
# - 对指定域名的 API 进行多次请求测试
# - 统计每次请求的 HTTP 状态码和响应时间
# - 计算平均响应时间和标准差
#
# 用法：time_test.sh <domain> <key> <count> [<model>]
#   domain - API 域名
#   key    - API Key
#   count  - 测试次数
#   model  - 模型名称（默认: gpt-3.5-turbo）

if [ $# -lt 3 ]; then
  echo "Usage: time_test.sh <domain> <key> <count> [<model>]"
  exit 1
fi

# 解析命令行参数
domain=$1
key=$2
count=$3
model=${4:-"gpt-3.5-turbo"} # 设置默认模型为 gpt-3.5-turbo

# 初始化统计变量
total_time=0
times=()

# 执行多次请求测试
for ((i=1; i<=count; i++)); do
  result=$(curl -o /dev/null -s -w "%{http_code} %{time_total}\\n" \
           https://"$domain"/v1/chat/completions \
           -H "Content-Type: application/json" \
           -H "Authorization: Bearer $key" \
           -d '{"messages": [{"content": "echo hi", "role": "user"}], "model": "'"$model"'", "stream": false, "max_tokens": 1}')
  http_code=$(echo "$result" | awk '{print $1}')
  time=$(echo "$result" | awk '{print $2}')
  echo "HTTP status code: $http_code, Time taken: $time"
  total_time=$(bc <<< "$total_time + $time")
  times+=("$time")
done

# 计算平均响应时间
average_time=$(echo "scale=4; $total_time / $count" | bc)

# 计算标准差
sum_of_squares=0
for time in "${times[@]}"; do
  difference=$(echo "scale=4; $time - $average_time" | bc)
  square=$(echo "scale=4; $difference * $difference" | bc)
  sum_of_squares=$(echo "scale=4; $sum_of_squares + $square" | bc)
done

standard_deviation=$(echo "scale=4; sqrt($sum_of_squares / $count)" | bc)

# 输出测试结果（平均时间 ± 标准差）
echo "Average time: $average_time±$standard_deviation"
