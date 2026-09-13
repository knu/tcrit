{{if eq .unresolved_count 1}}The review finished with 1 unresolved comment.{{else}}The review finished with {{.unresolved_count}} unresolved comments.{{end}}

{{if .comments_unresolved_json}}{{.comments_unresolved_json}}

{{end}}Read each thread's replies before acting.  Address new or changed reviewer feedback and reply once with the change or answer.  Leave unanswered agent comments and completion replies unchanged unless you have a material correction or additional result to report.  Unresolved status alone does not call for another reply.

{{if eq .internal_session_mode "plan"}}For needed replies, use `tcrit comment --plan {{.plan_slug}} --reply-to <id> --author <your-name> '<explanation>'`.{{else}}For needed replies, use `tcrit comment --session {{.session_key}} --reply-to <comment-id> --author <your-name> '<explanation>'`.{{end}}{{if .next_round_cmd}}

When you're done, run the next round even if no new reply was needed.  If the user cancelled or replaced this review task, stop its TUI instead; cancellation is not approval.

  {{.next_round_cmd}}{{end}}
