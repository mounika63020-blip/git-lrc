const { h, render } = window.preact;
const { useEffect, useMemo, useRef, useState } = window.preactHooks;

window.preact = { h, render, useEffect, useMemo, useRef, useState, html: window.htm.bind(h) };
