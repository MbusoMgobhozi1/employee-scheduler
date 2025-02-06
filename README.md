# Scheduling Assistant CSV Generator

This application generates staff schedules based on historical ticket volume data and operational constraints. It uses the OpenAI API (via the go-openai library) to produce a five‑week schedule in JSON format (as output from a Pandas DataFrame) and then converts that JSON output into week-specific CSV files.

## Overview

The scheduling assistant:
- Reads a CSV file (e.g., `Ticket-Volumes.csv`) containing historical ticket volume data.
- Computes the 75th percentile threshold for ticket volumes and determines the high-volume day numbers.
- Builds a prompt for ChatGPT including:
  - The high-volume day numbers.
  - A list of employee names.
  - Detailed operational constraints (such as shifts, maximum hours, rotation requirements, and forecast-driven staffing).
  - A requirement to generate a five‑week schedule with date-specific columns.
- Sends the prompt to ChatGPT via the OpenAI API.
- Receives a JSON array of schedule objects (each representing one employee's schedule for one week).
- Groups the JSON schedule by week and generates a CSV file for each week that only includes columns present in that week’s schedule.

## Features

- **Dynamic High-Volume Day Detection:**  
  Computes the 75th percentile of ticket volumes to determine which days have high call volume.

- **Flexible Prompt Generation:**  
  Builds a detailed prompt including operational constraints and date-specific column requirements.

- **ChatGPT Integration:**  
  Uses the OpenAI API to generate a schedule in JSON format.

- **Dynamic JSON-to-CSV Conversion:**  
  Extracts schedule data, groups it by week, and dynamically determines header columns based on keys present in each week’s data.

- **Per-Week CSV Files:**  
  Outputs separate CSV files for each week, ensuring that only the relevant columns for that week are included.

## Prerequisites

- [Go](https://golang.org/) (version 1.16 or higher recommended)
- An OpenAI API key with access to the appropriate model (set via the `OPENAI_API_KEY` environment variable)
- The `go-openai` library by [sashabaranov](https://github.com/sashabaranov/go-openai)

## Installation

1. **Clone the Repository:**

   ```bash
   git clone https://github.com/yourusername/scheduling-assistant.git
   cd scheduling-assistant
