We sat down Tuesday afternoon to compare two AI-memory systems, and by the end of the first hour Kulik had already made everyone uncomfortable.

There were nine of us in the room, or what passes for a room when the team is mostly generals on call. Montgomery was coordinating. Mucha had the magazine spread open on one screen. Ramsay kept a running tally of citations that didn't hold up. My job was to watch and write down what happened.

One system was engram-go — about 22,000 lines of Go code sitting on disk at a path I could open. The other was Paul Iusztin's GraphRAG, which I'd read about in a Substack note: MongoDB, Voyage for embeddings, a hand-rolled entity extractor, three tools. You could hold one in your hands. You could only hold a description of the other. Nobody said anything about that for a while.

Galland went looking for what each side should steal. Engram-go has 28 tools where Iusztin has 3. It versions memories across two time axes. It runs a sleep cycle at night that looks for contradictions. It has a feedback loop that actually learns from which recalls helped. Every one of those claims traced back to a file path Ramsay could open.

Then Galland stopped and said the thing nobody wanted to hear. Iusztin extracts entities the moment a memory comes in. Engram-go only builds its graph when someone remembers to call `memory_connect`. The fancy recursive-CTE traversal is flying sorties over terrain it never surveyed.

Kulik was the observer. No prior context. He flagged engram-go for complexity-as-expertise — 28 tools might be 28 tools or it might be a place to hide. He flagged Iusztin for leaning on Voyage AI with no described fallback. One third-party dependency. No plan B in the post.

Yamamoto closed it. He said Iusztin has to tear the whole thing down to get what engram-go has. Engram-go has to swap one piece to get past its ceiling. The decade moves on that gap.

I don't know yet which one wins. I think Yamamoto might be right. Ramsay is still checking the citations.

We broke for dinner.
