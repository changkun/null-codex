package main

import (
	notespkg "null-codex/pkg/notes"
	taskspkg "null-codex/tasks"
)

func fromNotesMetaSlice(items []notespkg.Meta) []noteMeta {
	result := make([]noteMeta, 0, len(items))
	for _, item := range items {
		result = append(result, noteMeta{
			ID:       item.ID,
			Title:    item.Title,
			Tags:     append([]string(nil), item.Tags...),
			Archived: item.Archived,
			ModTime:  item.ModTime,
		})
	}
	return result
}

func toNotesMetaSlice(items []noteMeta) []notespkg.Meta {
	result := make([]notespkg.Meta, 0, len(items))
	for _, item := range items {
		result = append(result, notespkg.Meta{
			ID:       item.ID,
			Title:    item.Title,
			Tags:     append([]string(nil), item.Tags...),
			Archived: item.Archived,
			ModTime:  item.ModTime,
		})
	}
	return result
}

func fromNotesContent(item notespkg.Content) noteContent {
	return noteContent{
		Title:       item.Title,
		Tags:        append([]string(nil), item.Tags...),
		Archived:    item.Archived,
		Attachments: fromNotesAttachments(item.Attachments),
		BodyLine:    item.BodyLine,
		Body:        item.Body,
	}
}

func toNotesContent(item noteContent) notespkg.Content {
	return notespkg.Content{
		Title:       item.Title,
		Tags:        append([]string(nil), item.Tags...),
		Archived:    item.Archived,
		Attachments: toNotesAttachments(item.Attachments),
		BodyLine:    item.BodyLine,
		Body:        item.Body,
	}
}

func fromNotesAttachment(item notespkg.Attachment) noteAttachment {
	return noteAttachment{Name: item.Name, StoredName: item.StoredName, MediaType: item.MediaType}
}

func toNotesAttachment(item noteAttachment) notespkg.Attachment {
	return notespkg.Attachment{Name: item.Name, StoredName: item.StoredName, MediaType: item.MediaType}
}

func fromNotesAttachments(items []notespkg.Attachment) []noteAttachment {
	result := make([]noteAttachment, 0, len(items))
	for _, item := range items {
		result = append(result, fromNotesAttachment(item))
	}
	return result
}

func toNotesAttachments(items []noteAttachment) []notespkg.Attachment {
	result := make([]notespkg.Attachment, 0, len(items))
	for _, item := range items {
		result = append(result, toNotesAttachment(item))
	}
	return result
}

func fromNotesEdges(items []notespkg.Edge) []noteEdge {
	result := make([]noteEdge, 0, len(items))
	for _, item := range items {
		result = append(result, noteEdge{Source: item.Source, Target: item.Target})
	}
	return result
}

func fromBrokenLinks(items []notespkg.BrokenLink) []brokenLink {
	result := make([]brokenLink, 0, len(items))
	for _, item := range items {
		result = append(result, brokenLink{Source: item.Source, Target: item.Target})
	}
	return result
}

func fromTask(item taskspkg.Task) noteTask {
	return noteTask{
		Text:      item.Text,
		RawText:   item.RawText,
		Line:      item.Line,
		Open:      item.Open,
		Source:    item.Source,
		DueDate:   item.DueDate,
		DueTime:   item.DueTime,
		DueStatus: item.DueStatus,
	}
}

func toTask(item noteTask) taskspkg.Task {
	return taskspkg.Task{
		Text:      item.Text,
		RawText:   item.RawText,
		Line:      item.Line,
		Open:      item.Open,
		Source:    item.Source,
		DueDate:   item.DueDate,
		DueTime:   item.DueTime,
		DueStatus: item.DueStatus,
	}
}

func fromTasks(items []taskspkg.Task) []noteTask {
	result := make([]noteTask, 0, len(items))
	for _, item := range items {
		result = append(result, fromTask(item))
	}
	return result
}
