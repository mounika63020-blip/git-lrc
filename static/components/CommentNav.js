// CommentNav component - floating prev/next comment navigator
import { waitForPreact } from './utils.js';

export async function createCommentNav() {
    const { html, useState, useEffect, useCallback } = await waitForPreact();

    return function CommentNav({ allComments, commentKey, onNavigate, activeTab }) {
        const [currentIdx, setCurrentIdx] = useState(-1);

        // Reset index only when the actual comment set changes (not on every render)
        useEffect(() => {
            setCurrentIdx(-1);
        }, [commentKey]);

        // Clamp index if comments shrink
        useEffect(() => {
            if (currentIdx >= allComments.length) {
                setCurrentIdx(allComments.length > 0 ? allComments.length - 1 : -1);
            }
        }, [allComments.length, currentIdx]);

        const goTo = useCallback((idx) => {
            if (allComments.length === 0) return;
            setCurrentIdx(idx);
            const c = allComments[idx];
            onNavigate(c.commentId, c.fileId);
        }, [allComments, onNavigate]);

        const goNext = useCallback(() => {
            if (allComments.length === 0) return;
            const next = currentIdx < allComments.length - 1 ? currentIdx + 1 : 0;
            goTo(next);
        }, [allComments.length, currentIdx, goTo]);

        const goPrev = useCallback(() => {
            if (allComments.length === 0) return;
            const prev = currentIdx > 0 ? currentIdx - 1 : allComments.length - 1;
            goTo(prev);
        }, [allComments.length, currentIdx, goTo]);

        // Keyboard shortcuts: j = next, k = prev
        useEffect(() => {
            const handler = (e) => {
                // Ignore if typing in an input/textarea
                const tag = (e.target.tagName || '').toLowerCase();
                if (tag === 'input' || tag === 'textarea' || tag === 'select') return;
                if (e.target.isContentEditable) return;
                // Only active on files tab
                if (activeTab !== 'files') return;
                if (allComments.length === 0) return;

                if (e.key === 'j' || e.key === 'J') {
                    e.preventDefault();
                    goNext();
                } else if (e.key === 'k' || e.key === 'K') {
                    e.preventDefault();
                    goPrev();
                }
            };
            document.addEventListener('keydown', handler);
            return () => document.removeEventListener('keydown', handler);
        }, [activeTab, allComments.length, goNext, goPrev]);

        // Hide when no comments or not on files tab
        if (allComments.length === 0 || activeTab !== 'files') return null;

        const display = currentIdx >= 0
            ? `${currentIdx + 1} / ${allComments.length}`
            : `— / ${allComments.length}`;

        return html`
            <div class="comment-nav">
                <button
                    class="comment-nav-btn"
                    onClick=${goPrev}
                    title="Previous comment (k)"
                    aria-label="Previous comment"
                >
                    <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round">
                        <polyline points="15 18 9 12 15 6" />
                    </svg>
                </button>
                <span class="comment-nav-counter">${display}</span>
                <button
                    class="comment-nav-btn"
                    onClick=${goNext}
                    title="Next comment (j)"
                    aria-label="Next comment"
                >
                    <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round">
                        <polyline points="9 18 15 12 9 6" />
                    </svg>
                </button>
            </div>
        `;
    };
}

let CommentNavComponent = null;
export async function getCommentNav() {
    if (!CommentNavComponent) {
        CommentNavComponent = await createCommentNav();
    }
    return CommentNavComponent;
}
