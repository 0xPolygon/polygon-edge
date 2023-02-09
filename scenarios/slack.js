import http from "k6/http";
import { textSummary } from 'https://jslib.k6.io/k6-summary/0.0.2/index.js';

export const payload = {
  "blocks": [
    {
      "type": "header",
      "text": {
        "type": "plain_text",
        "text": "Pandora's Box Results"
      }
    },
    {
      "type": "section",
      "text": {
        "type": "mrkdwn",
        "text": ""
      }
    },
    {
      "type": "section",
      "text": {
        "type": "mrkdwn",
        "text": ""
      }
    },
    {
      "type": "divider"
    },
    {
      "type": "context",
      "elements": [
        {
          "type": "mrkdwn",
          "text": ""
        },
        {
          "type": "mrkdwn",
          "text": ""
        },
        {
          "type": "mrkdwn",
          "text": ""
        },
        {
          "type": "mrkdwn",
          "text": ""
        }
      ]
    }
  ]
}

export function sendSlackMessage(data) {
  payload.blocks[1].text.text = "Approximate TPS:" + data.ethereum_tps
  payload.blocks[2].text.text = "Total Transactions: " + data.iterations

  const url = __ENV.SLACK_WEBHOOK_URL;
  const payload = JSON.stringify(data);
  const params = {
    headers: {
      "Content-Type": "application/json",
    },
  };
  const slackRes = http.post(url, payload, params);
  console.log(slackRes.body);
}
