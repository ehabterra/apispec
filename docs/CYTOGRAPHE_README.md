# apispec

**License:** This project is licensed under the Apache License 2.0, Copyright 2025 Ehab Terra. See the [LICENSE](./LICENSE) and [NOTICE](./NOTICE) files for details.

# Cytoscape.js Call Tree Diagram

This implementation provides a **tree-based call diagram** using Cytoscape.js that displays function call relationships in a hierarchical tree structure, similar to Mermaid but with better performance for large diagrams.

## Key Features

### 🎯 **Tree-Based Layout**
- **Hierarchical Structure**: Functions are arranged in a top-to-bottom tree layout
- **Fixed Positioning**: Nodes are positioned automatically and cannot be dragged
- **Rectangular Nodes**: Clean, professional rectangular nodes instead of circles
- **Clear Hierarchy**: Easy to trace function call chains from root to leaves

### 🎨 **Visual Design**
- **Rectangular Nodes**: Professional rectangular shape with rounded corners
- **Color Coding**: 
  - Blue: Regular functions
  - Red: Root functions (entry points)
  - Green: Function call edges
- **Clean Typography**: Readable labels with proper text wrapping
- **Modern UI**: Gradient background with clean controls

### 🚀 **Performance**
- **Optimized for Large Trees**: Handles thousands of nodes efficiently
- **Smooth Animations**: Fluid layout transitions
- **Fast Rendering**: Cytoscape.js provides excellent performance

### 🎮 **Interactive Features**
- **Click to Highlight**: Click any node to highlight its call chain
- **Zoom & Pan**: Navigate large trees easily
- **Layout Options**: Switch between different tree layouts
- **Export**: Save as PNG for documentation

## Controls

### Layout Options
- **Tree Layout (Dagre)**: Recommended hierarchical layout
- **Breadth-First Tree**: Alternative tree structure
- **Grid Layout**: Simple grid arrangement

### Interactive Controls
- **Reset**: Return to default view
- **Fit View**: Automatically fit all nodes to screen
- **Toggle Labels**: Show/hide node labels
- **Expand Tree**: Show all nodes
- **Collapse Tree**: Show only root and direct children
- **Export PNG**: Save diagram as image

### Keyboard Shortcuts
- `R`: Reset view
- `F`: Fit to view
- `L`: Toggle labels
- `E`: Expand tree
- `C`: Collapse tree

## Usage

The diagram is automatically generated when you run the apispec tool:

```bash
./apispec -config your-config.yaml
```

This creates `diagram.html` which you can open in any web browser.

## Technical Details

### Layout Algorithm
- Uses **Dagre** layout engine for optimal tree positioning
- **Top-to-bottom** flow direction
- **Automatic spacing** between nodes and levels
- **Hierarchical ranking** for clear call chains

### Node Styling
- **Rectangular shape** with rounded corners
- **Fixed dimensions**: 120x50px for regular nodes
- **Text wrapping** for long function names
- **Border and shadow** effects for depth

### Edge Styling
- **Directed arrows** showing call direction
- **Curved lines** for better visual flow
- **Thick edges** (3px) for better visibility
- **Color-coded** for easy identification

## Advantages Over Mermaid

1. **Performance**: Much faster rendering for large diagrams
2. **Interactivity**: Click to highlight, zoom, pan
3. **Flexibility**: Multiple layout options
4. **Professional Look**: Modern UI with better styling
5. **Export Options**: Save as high-quality PNG
6. **Tree Structure**: Clear hierarchical layout
7. **Fixed Positioning**: No accidental node movement

## Browser Compatibility

Works in all modern browsers:
- Chrome/Chromium
- Firefox
- Safari
- Edge

## File Structure

```
apispec/
├── internal/spec/mapper.go    # Cytoscape generation logic
├── diagram.html              # Generated interactive diagram
└── CYTOGRAPHE_README.md      # This documentation
```

The implementation is integrated into the existing `MapMetadataToOpenAPI` function and automatically generates the diagram alongside the OpenAPI specification. 