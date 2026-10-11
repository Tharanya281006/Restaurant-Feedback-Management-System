# Restaurant Feedback Management System

A small Go web application for collecting and reviewing customer feedback for one restaurant.

## Features
- Customer feedback form with optional anonymous submission.
- Overall 1–5 star rating and six restaurant experience ratings: Food Quality, Service Speed, Staff Behaviour, Cleanliness, Ambience, and Value for Money.
- Optional visit date and written feedback/suggestions.
- Show All Feedback table with search, pagination, and submission date.
- Dashboard statistics: total feedback, average overall rating, positive ratings (4–5), and ratings needing improvement (1–2).
- Feedback is stored in `feedback-data.json` and loaded on application start, so it persists across restarts.
- No Accepted/Updated status column and no Track Response screen.

## Run
Install a Go version that satisfies the version declared in `go.mod`, open a terminal in this folder, and run:

```powershell
go run .
```

Then visit http://localhost:8080.

## Data storage
`feedback-data.json` is the local data file. Keep it in the project folder and do not delete it if you want to retain submitted feedback. Back it up regularly. Data is local to this machine unless the file and application are moved together.
