{
  "displayName": "${title}: ${deployment_name}",
  "gridLayout": {
    "columns": 2,
    "widgets": [
      {
        "text": {
          "content": "Metrics from the ${deployment_name} deployment of the Cluster Toolkit.",
          "format": "MARKDOWN"
        },
        "title": "${title}"
      }%{ for widget in widgets ~},
      ${widget}
      %{endfor ~}
    ]
  }
}
