{{if eq .unresolved_count 1}}The review finished with 1 unresolved comment.{{else}}The review finished with {{.unresolved_count}} unresolved comments.{{end}}

{{if .comments_unresolved_json}}{{.comments_unresolved_json}}

{{end}}Address each comment. For each one, reply explaining what you did using `tcrit comment --reply-to <comment-id> --author <your-name> "<explanation>"`.{{if .next_round_cmd}}

When you're done, run:

  {{.next_round_cmd}}{{end}}
