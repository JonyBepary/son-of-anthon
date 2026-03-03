# Nextcloud Deck Kanban Integration - Update Summary

## Changes Implemented ✅

### 1. Three-Stack Kanban System
- **Want To Learn** (Blue - `0800fd`) - For courses with 0% progress
- **In Progress** (Orange - `ff6f00`) - For courses with 1-99% progress  
- **Completed** (Green - `31cc7c`) - For courses with 100% progress

### 2. Auto-Move Functionality
Cards automatically move between stacks based on progress:
- When you start a course (0% → 1%): moves to "In Progress"
- When you complete a course (99% → 100%): moves to "Completed"
- The system searches all stacks for existing cards and moves them to the correct stack

### 3. Enhanced Card Descriptions
Each card now displays:
```
📚 **Course Name**

**Progress:** 12/16 units (75%)
**Type:** Book

📊 **Weekly Progress:**
Week 1: ███░░░░ 3 units
Week 2: █████░░ 5 units
Week 3: ████░░░ 4 units
Week 4: ██░░░░░ 2 units

📈 **Monthly Summary:**
This month: 14 units

⚡ **Velocity:** 3.5 units/week
```

### 4. Smart Labels
Auto-generated labels based on:
- **Course Type**: Book, Video, Course
- **Keywords**: Detects and adds labels for:
  - AI/ML (deep learning, machine learning)
  - Programming languages (Python, JavaScript)
  - Frameworks (React)
  - Subjects (IELTS, Mathematics, Algorithms)

### 5. Automatic Stack Creation
When you run `sync_deck`, the system automatically:
- Creates the 3 stacks if they don't exist
- Uses the correct colors for each stack
- Maintains existing stacks if already present

## Files Modified

1. **`pkg/skills/coach/nextcloud.go`**
   - Added `ensureKanbanStacks()` - Creates/finds the 3 stacks
   - Added `createStack()` - Creates individual stacks with colors
   - Added `findOrCreateDeckCardInStack()` - Finds cards across all stacks
   - Added `moveCardToStack()` - Moves cards between stacks
   - Added `buildProgressDescription()` - Generates rich progress visualization
   - Added `buildCourseLabels()` - Generates smart labels
   - Updated `executeSyncDeck()` - Main sync logic with auto-move

2. **`pkg/skills/coach/skill.go`**
   - Updated tool description

3. **`workspaces/coach/TOOLS.md`**
   - Updated documentation with new features

## Usage

Simply run the sync command as before:
```
sync_deck
```

The Coach agent will:
1. Create the 3 Kanban stacks (if needed)
2. Sync all active courses as cards
3. Place each card in the correct stack based on progress
4. Update card descriptions with progress charts
5. Add relevant labels

## Example Output
```
✅ Synced 5 courses to Nextcloud Deck
```

Your Nextcloud Deck board will now show:
- **Want To Learn**: New courses you haven't started
- **In Progress**: Courses you're actively working on (with progress bars)
- **Completed**: Finished courses (with completion stats)

## Next Steps

To test:
1. Rebuild the project: `cd ~/pico-son-of-anthon && make build`
2. Restart the Coach agent
3. Run: `sync_deck`
4. Check your Nextcloud Deck board!

---
*Updated: 2026-03-03*
