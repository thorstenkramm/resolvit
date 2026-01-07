# Markdown

## Markdown Standards

1. Lines should not exceed a maximum length of 120 characters.
2. Use proper headings. Do not use bold type to create headings.  
   Bad:

   ```markdown
   ## Heading 2

    - **Something Important:** Some text that refers to something important. And so and so on bla bla
   ```

   Good:

   ```markdown
   ## Heading 2

   ### Something Important

   Some text that refers to something important. And so and so on bla bla
   ```

3. Always lint Markdown files using `npx markdownlint --fix <FILE>.md` and fix findings until linter succeeds.
4. Avoid large Markdown tables. Do not use tables when cells have line breaks or content larger than 75 characters.
   Small tables are ok.
5. Use ordered and unordered lists only when they make sense.
6. Use a blank line to separate logical blocks inside a text or chapter.
7. Use double blank space to force a line break.
8. Keep in mind; users will read the rendered markdown, not the source. Both need to be beautiful: Source and rendered result.
9. Using GitHub Flavored Markdown (GLFM) is possible [Specs](https://docs.github.com/de/get-started/writing-on-github/getting-started-with-writing-and-formatting-on-github/basic-writing-and-formatting-syntax).
10. Use [Alerts](https://docs.github.com/de/get-started/writing-on-github/getting-started-with-writing-and-formatting-on-github/basic-writing-and-formatting-syntax#alerts)
    to draw attention to important parts.
11. Ordered or unordered lists with just one item are pointless. Avoid lists with one item.

## Examples

### Bad

```markdown
### Some header for this chapter

1. **This is something I need to tell you:**
    - Oh acceptance apartments up sympathize astonished **delightful**. Waiting him new lasting towards. Continuing
      melancholy especially so to. Me unpleasing impossible in attachment announcing so astonished.
      What ask leaf may nor upon door. Tended remain my do stairs. Oh smiling amiable am so visited cordial in offices
      hearted.

2. **Consider this too:**
    - Answer misery adieus add wooded how nay men before though. **Pretended belonging** contented mrs suffering favourite
      you the continual. Mrs civil nay least means tried drift. Natural end law whether but and towards certain.
      Furnished unfeeling his sometimes see day promotion.

3. **Next Chapter:**
    - At every tiled on ye defer do. No attention suspected oh difficult. Fond his say old meet cold find come whom. 
```

Problems:

1. Boldtype used as heading
2. Single item lists

### Good

```markdown
### Some header for this chapter

#### 1. This is something I need to tell you
Oh acceptance apartments up sympathize astonished **delightful**. Waiting him new lasting towards. Continuing
melancholy especially so to. Me unpleasing impossible in attachment announcing so astonished.
What ask leaf may nor upon door. Tended remain my do stairs. Oh smiling amiable am so visited cordial in offices
hearted.

#### 2. Consider this too
Answer misery adieus add wooded how nay men before though. **Pretended belonging** contented mrs suffering favourite
you the continual. Mrs civil nay least means tried drift. Natural end law whether but and towards certain.
Furnished unfeeling his sometimes see day promotion.

#### 3. **Next Chapter
At every tiled on ye defer do. No attention suspected oh difficult. Fond his say old meet cold find come whom. 
```
