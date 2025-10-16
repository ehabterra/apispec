#!/bin/bash

# APISpec Metrics Viewer Script
# Provides easy access to different visualization methods

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"

# Use provided metrics file or default
if [ -z "$1" ]; then
    METRICS_FILE="$PROJECT_ROOT/profiles/metrics.json"
else
    METRICS_FILE="$1"
fi

echo "APISpec Metrics Viewer"
echo "======================"
echo "Metrics file: $METRICS_FILE"
echo ""

if [ ! -f "$METRICS_FILE" ]; then
    echo "Error: Metrics file not found at $METRICS_FILE"
    echo "Usage: $0 [path/to/metrics.json]"
    echo ""
    echo "To generate metrics, run apispec with --custom-metrics flag:"
    echo "  ./apispec --dir ./myproject --output spec.yaml --custom-metrics"
    exit 1
fi

echo "Choose visualization method:"
echo "1) HTML Viewer (opens in browser)"
echo "2) Quick Summary (text only)"
echo "3) JSON Pretty Print"
echo "4) Raw JSON"
echo "5) Choose different metrics file"
echo ""
read -p "Enter choice (1-5): " choice

case $choice in
    1)
        echo "Opening HTML viewer..."
        if command -v python3 &> /dev/null; then
            cd "$PROJECT_ROOT"
            python3 -m http.server 8000 &
            SERVER_PID=$!
            sleep 2
            
            # Get relative path from project root to metrics file
            # Convert to absolute path first if it's relative
            if [[ "$METRICS_FILE" == /* ]]; then
                # Already absolute path
                ABSOLUTE_METRICS_FILE="$METRICS_FILE"
            else
                # Relative path, make it absolute from current directory
                ABSOLUTE_METRICS_FILE="$(pwd)/$METRICS_FILE"
            fi
            
            # Now get relative path from project root
            if [[ "$ABSOLUTE_METRICS_FILE" == "$PROJECT_ROOT"* ]]; then
                # File is within project root, get relative path
                METRICS_REL_PATH="${ABSOLUTE_METRICS_FILE#$PROJECT_ROOT/}"
            else
                # File is outside project root, use absolute path
                METRICS_REL_PATH="$ABSOLUTE_METRICS_FILE"
            fi
            
            # URL encode the file path to handle special characters
            METRICS_REL_PATH_ENCODED=$(printf '%s\n' "$METRICS_REL_PATH" | sed 's/ /%20/g')
            VIEWER_URL="http://localhost:8000/scripts/metrics_viewer.html?file=$METRICS_REL_PATH_ENCODED"
            
            echo "Opening browser at $VIEWER_URL"
            if command -v open &> /dev/null; then
                open "$VIEWER_URL"
            elif command -v xdg-open &> /dev/null; then
                xdg-open "$VIEWER_URL"
            else
                echo "Please open $VIEWER_URL in your browser"
            fi
            echo "Press Ctrl+C to stop the server"
            
            # Set up signal handling to clean up the server
            cleanup() {
                echo ""
                echo "Stopping server..."
                kill $SERVER_PID 2>/dev/null
                wait $SERVER_PID 2>/dev/null
                echo "Server stopped."
                exit 0
            }
            
            # Trap termination signals
            trap cleanup SIGINT SIGTERM
            
            # Wait for the server process
            wait $SERVER_PID
        else
            echo "Python3 not found. Please open $SCRIPT_DIR/metrics_viewer.html in your browser"
            echo "To load your metrics file directly, add ?file=path/to/your/metrics.json to the URL"
        fi
        ;;
    2)
        echo "Quick Summary:"
        echo "=============="
        if command -v jq &> /dev/null; then
            echo "Peak Memory: $(jq -r '.[] | select(.name == "memory.alloc") | .value' "$METRICS_FILE" | sort -n | tail -1 | numfmt --to=iec)"
            echo "Total Allocations: $(jq -r '.[] | select(.name == "memory.total_alloc") | .value' "$METRICS_FILE" | tail -1 | numfmt --to=iec)"
            echo "GC Collections: $(jq -r '.[] | select(.name == "gc.num_gc") | .value' "$METRICS_FILE" | tail -1)"
            echo "Generation Time: $(jq -r '.[] | select(.name == "openapi_generation") | .value' "$METRICS_FILE" | head -1 | awk '{print $1/1000000000 " seconds"}')"
        else
            echo "jq not found. Please install jq for better summary display."
            echo "Basic summary using awk:"
            echo "File size: $(wc -c < "$METRICS_FILE") bytes"
            echo "Number of metrics: $(grep -c '"name"' "$METRICS_FILE")"
        fi
        ;;
    3)
        echo "Pretty printing JSON..."
        if command -v jq &> /dev/null; then
            jq . "$METRICS_FILE"
        else
            echo "jq not found. Please install jq for JSON pretty printing."
            echo "Raw JSON content:"
            cat "$METRICS_FILE"
        fi
        ;;
    4)
        echo "Raw JSON:"
        cat "$METRICS_FILE"
        ;;
    5)
        echo "Enter metrics filename (e.g., chi-metrics.json, fiber-metrics.json, metrics.json):"
        read -p "Filename: " filename
        if [ -z "$filename" ]; then
            echo "No filename provided"
            exit 1
        fi
        # Check in profiles directory first, then current directory
        if [ -f "$PROJECT_ROOT/profiles/$filename" ]; then
            new_file="$PROJECT_ROOT/profiles/$filename"
        elif [ -f "$filename" ]; then
            new_file="$filename"
        else
            echo "Error: File '$filename' not found in profiles/ or current directory"
            exit 1
        fi
        echo "Switching to: $new_file"
        exec "$0" "$new_file"
        ;;
    *)
        echo "Invalid choice"
        exit 1
        ;;
esac
